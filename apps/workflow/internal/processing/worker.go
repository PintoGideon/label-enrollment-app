package processing

import (
	"context"
	"errors"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/database"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

// Work is restartable: persisted intent is authoritative; there is no in-memory
// queue whose disappearance loses an accepted command. A lease fence protects
// every result/progress commit. Shutdown does not cancel accepted processing.
func (s *Service) Work(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}
func (s *Service) tick(ctx context.Context) {
	claim, e := s.Store.ClaimProcessing(ctx)
	if e != nil || claim == nil {
		return
	}
	step, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	stopped := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stopped:
				return
			case <-step.Done():
				return
			case <-t.C:
				if !s.Store.RenewProcessing(step, claim) {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		close(stopped)
		<-done
		cleanup, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		s.Store.ReleaseProcessing(cleanup, claim)
	}()
	r := claim.Run
	if r.Attempt != nil {
		attempt := *r.Attempt
		r.Attempt = &attempt
	}
	// Revocation is checked from durable human intent, not an expired token or
	// privileged runner credential. Database unavailability is not a denial.
	if r.State != "cancelling" {
		e = s.Store.ProcessingAuthority(step, r)
		if errors.Is(e, database.ErrForbidden) || errors.Is(e, database.ErrNotFound) {
			r.State = "cancelling"
			r.FailureCode = "AUTHORITY_REVOKED"
		} else if e != nil {
			return
		}
	}
	switch r.State {
	case "verifying":
		input, e := s.verifyInput(step, r)
		if e == nil {
			r.Input = input
			r.State = "verified"
			r.FailureCode = ""
			r.Progress = api.Progress{Stage: "input_verified", Frames: len(input.Objects)}
		} else if errors.Is(e, errInputInvalid) {
			r.State = "failed"
			r.FailureCode = "INPUT_INVALID"
		} else {
			r.FailureCode = "DEPENDENCY_UNAVAILABLE"
		}
	case "cancelling":
		if r.Attempt == nil {
			r.State = "cancelled"
			r.Progress.Stage = "cancelled"
		} else {
			s.executionStep(step, &r)
		}
	case "queued", "running", "uncertain":
		s.executionStep(step, &r)
	case "validating":
		result, e := s.importResult(step, r)
		if e == nil {
			r.Result = result
			r.State = "reviewable"
			r.Progress.Stage = "reviewable"
			r.FailureCode = ""
		} else {
			r.State = "failed"
			r.FailureCode = "RESULT_INVALID"
			if errors.Is(e, errReferenceRequired) {
				r.FailureCode = "REFERENCE_REQUIRED"
			}
		}
	}
	if step.Err() == nil && api.Digest(r) != api.Digest(claim.Run) {
		_ = s.Store.SaveProcessing(step, claim, r)
	}
}
