package database

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/auth"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/database/dbsql"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrProcessingUnavailable = errors.New("Workflow processing is unavailable")
	ErrNotFound              = errors.New("Workflow resource not found")
	ErrForbidden             = errors.New("Workflow action is not permitted")
	ErrConflict              = errors.New("Workflow command conflicts with current state")
	ErrBadCommand            = errors.New("invalid Workflow command")
)

func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func uuid(s string) pgtype.UUID { var v pgtype.UUID; _ = v.Scan(s); return v }
func encoded(v any) []byte      { b, _ := json.Marshal(v); return b }
func readRun(b []byte) (api.Run, error) {
	var r api.Run
	if json.Unmarshal(b, &r) != nil || !r.Valid() {
		return r, ErrProcessingUnavailable
	}
	return r, nil
}
func safeProcessingError(err error) error {
	if err == nil {
		return nil
	}
	for _, e := range []error{ErrNotFound, ErrForbidden, ErrConflict, ErrBadCommand} {
		if errors.Is(err, e) {
			return e
		}
	}
	return ErrProcessingUnavailable
}
func authorize(ctx context.Context, q *dbsql.Queries, project string, p auth.Principal, role string) error {
	if !p.Valid() {
		return ErrForbidden
	}
	roles, e := q.ProcessingRoles(ctx, dbsql.ProcessingRolesParams{ProjectID: uuid(project), Issuer: p.Issuer, Subject: p.Subject})
	if e != nil {
		return ErrProcessingUnavailable
	}
	if len(roles) == 0 {
		return ErrNotFound
	}
	if role != "" && !slices.Contains(roles, role) {
		return ErrForbidden
	}
	return nil
}
func (s *Store) transaction(ctx context.Context, fn func(*dbsql.Queries) error) error {
	if s == nil || s.Check(ctx) != api.DatabaseReady {
		return ErrProcessingUnavailable
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return ErrProcessingUnavailable
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = tx.Rollback(cleanup)
	}()
	if e = fn(dbsql.New(tx)); e != nil {
		return safeProcessingError(e)
	}
	// A lost COMMIT acknowledgement is uncertain; clients replay the same key.
	return safeProcessingError(tx.Commit(ctx))
}

// ApplyProcessingCommand serializes project mutations, current membership,
// optimistic run revisions, event/history writes and exact response receipts.
func (s *Store) ApplyProcessingCommand(ctx context.Context, p auth.Principal, project, id, key, role, action string, payload any, change func(*api.Run) error) (result api.Run, err error) {
	if !p.Human() {
		return result, ErrForbidden
	}
	digest := api.Digest(struct {
		Action, Run string
		Payload     any
	}{action, id, payload})
	err = s.transaction(ctx, func(q *dbsql.Queries) error {
		// Check membership before locking/looking up anything externally observable.
		if e := authorize(ctx, q, project, p, role); e != nil {
			return e
		}
		if _, e := q.LockProcessingProject(ctx, uuid(project)); e != nil {
			return e
		}
		receipt, e := q.FindProcessingCommand(ctx, dbsql.FindProcessingCommandParams{ProjectID: uuid(project), Issuer: p.Issuer, Subject: p.Subject, CommandID: uuid(key)})
		if e == nil {
			if receipt.RequestDigest != digest {
				return ErrConflict
			}
			result, e = readRun(receipt.Response)
			return e
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		var before api.Run
		if id != "" {
			b, e := q.LockProcessingRun(ctx, dbsql.LockProcessingRunParams{ProjectID: uuid(project), ID: uuid(id)})
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrNotFound
			}
			if e != nil {
				return e
			}
			before, e = readRun(b)
			if e != nil {
				return e
			}
			if json.Unmarshal(b, &result) != nil {
				return ErrProcessingUnavailable
			}
		} else {
			result = api.Run{ID: NewID(), ProjectID: project, ApprovalIDs: []string{}}
		}
		if e = change(&result); e != nil {
			return e
		}
		result.Revision = before.Revision + 1
		if !result.Valid() {
			return ErrProcessingUnavailable
		}
		if id == "" {
			e = q.CreateProcessingRun(ctx, dbsql.CreateProcessingRunParams{ID: uuid(result.ID), ProjectID: uuid(project), Revision: result.Revision, State: result.State, OwnerIssuer: result.Owner.Issuer, OwnerSubject: result.Owner.Subject, Snapshot: encoded(result)})
		} else {
			var n int64
			n, e = q.UpdateProcessingRun(ctx, dbsql.UpdateProcessingRunParams{ID: uuid(id), Revision: before.Revision, State: result.State, Snapshot: encoded(result)})
			if e == nil && n != 1 {
				return ErrConflict
			}
		}
		if e != nil {
			return e
		}
		actor := api.Actor{Issuer: p.Issuer, Subject: p.Subject}
		if e = saveProcessingHistory(ctx, q, before, result, action, &actor); e != nil {
			return e
		}
		return q.SaveProcessingCommand(ctx, dbsql.SaveProcessingCommandParams{ProjectID: uuid(project), Issuer: p.Issuer, Subject: p.Subject, CommandID: uuid(key), RequestDigest: digest, Response: encoded(result)})
	})
	return result, err
}
func saveProcessingHistory(ctx context.Context, q *dbsql.Queries, before, r api.Run, kind string, actor *api.Actor) error {
	if r.Attempt != nil && (before.Attempt == nil || before.Attempt.ID != r.Attempt.ID) {
		if e := q.SaveProcessingAttempt(ctx, dbsql.SaveProcessingAttemptParams{ID: uuid(r.Attempt.ID), RunID: uuid(r.ID), Number: int32(r.Attempt.Number), Intent: encoded(r.Attempt)}); e != nil {
			return e
		}
	}
	if r.Result != nil && (before.Result == nil || before.Result.Digest != r.Result.Digest) {
		if e := q.SaveProcessingResult(ctx, dbsql.SaveProcessingResultParams{AttemptID: uuid(r.Result.AttemptID), Result: encoded(r.Result)}); e != nil {
			return e
		}
	}
	if r.Review != nil && (before.Review == nil || before.Review.ID != r.Review.ID) {
		if e := q.SaveProcessingReview(ctx, dbsql.SaveProcessingReviewParams{ID: uuid(r.Review.ID), RunID: uuid(r.ID), Review: encoded(r.Review)}); e != nil {
			return e
		}
	}
	if r.Approval != nil && (before.Approval == nil || before.Approval.ID != r.Approval.ID) {
		if e := q.SaveProcessingApproval(ctx, dbsql.SaveProcessingApprovalParams{ID: uuid(r.Approval.ID), RunID: uuid(r.ID), Approval: encoded(r.Approval)}); e != nil {
			return e
		}
	}
	event := api.RunEvent{RunID: r.ID, Revision: r.Revision, Kind: kind, Actor: actor, State: r.State, Progress: r.Progress, CreatedAt: time.Now().UTC()}
	return q.SaveProcessingEvent(ctx, dbsql.SaveProcessingEventParams{RunID: uuid(r.ID), Revision: r.Revision, Event: encoded(event)})
}
func (s *Store) ReadRun(ctx context.Context, p auth.Principal, project, id string) (r api.Run, err error) {
	err = s.transaction(ctx, func(q *dbsql.Queries) error {
		if e := authorize(ctx, q, project, p, ""); e != nil {
			return e
		}
		b, e := q.ReadProcessingRun(ctx, dbsql.ReadProcessingRunParams{ProjectID: uuid(project), ID: uuid(id)})
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		r, e = readRun(b)
		return e
	})
	return
}
func (s *Store) ReadRunEvents(ctx context.Context, p auth.Principal, project, id string, after int64) (page api.RunEventPage, err error) {
	page.Events = []api.RunEvent{}
	err = s.transaction(ctx, func(q *dbsql.Queries) error {
		if e := authorize(ctx, q, project, p, ""); e != nil {
			return e
		}
		if _, e := q.ReadProcessingRun(ctx, dbsql.ReadProcessingRunParams{ProjectID: uuid(project), ID: uuid(id)}); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return e
		}
		rows, e := q.ListProcessingEvents(ctx, dbsql.ListProcessingEventsParams{RunID: uuid(id), Sequence: after})
		if e != nil {
			return e
		}
		more := len(rows) > 100
		if more {
			rows = rows[:100]
		}
		for _, row := range rows {
			var event api.RunEvent
			if json.Unmarshal(row.Event, &event) != nil {
				return ErrProcessingUnavailable
			}
			event.Sequence = row.Sequence
			page.Events = append(page.Events, event)
		}
		if more {
			page.NextCursor = rows[len(rows)-1].Sequence
		}
		return nil
	})
	return
}
func (s *Store) ReadApproval(ctx context.Context, p auth.Principal, project, id, approval string) (result api.ProcessingApproval, err error) {
	err = s.transaction(ctx, func(q *dbsql.Queries) error {
		if e := authorize(ctx, q, project, p, ""); e != nil {
			return e
		}
		if _, e := q.ReadProcessingRun(ctx, dbsql.ReadProcessingRunParams{ProjectID: uuid(project), ID: uuid(id)}); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return e
		}
		b, e := q.ReadProcessingApproval(ctx, dbsql.ReadProcessingApprovalParams{RunID: uuid(id), ID: uuid(approval)})
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		return json.Unmarshal(b, &result)
	})
	return
}

func (s *Store) ReadResult(ctx context.Context, p auth.Principal, project, id, attempt string) (result api.ProcessingResult, err error) {
	err = s.transaction(ctx, func(q *dbsql.Queries) error {
		if e := authorize(ctx, q, project, p, ""); e != nil {
			return e
		}
		if _, e := q.ReadProcessingRun(ctx, dbsql.ReadProcessingRunParams{ProjectID: uuid(project), ID: uuid(id)}); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return e
		}
		b, e := q.ReadProcessingResult(ctx, dbsql.ReadProcessingResultParams{RunID: uuid(id), ID: uuid(attempt)})
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if json.Unmarshal(b, &result) != nil || result.Digest != result.SealedDigest() {
			return ErrProcessingUnavailable
		}
		return nil
	})
	return
}

type ProcessingClaim struct {
	Run   api.Run
	Owner string
	Fence int64
}

func (s *Store) ClaimProcessing(ctx context.Context) (claim *ProcessingClaim, err error) {
	if s == nil || s.Check(ctx) != api.DatabaseReady {
		return nil, ErrProcessingUnavailable
	}
	owner := NewID()
	row, e := s.queries.ClaimProcessingRun(ctx, uuid(owner))
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, ErrProcessingUnavailable
	}
	r, e := readRun(row.Snapshot)
	if e != nil {
		return nil, e
	}
	return &ProcessingClaim{Run: r, Owner: owner, Fence: row.Fence}, nil
}
func (s *Store) RenewProcessing(ctx context.Context, c *ProcessingClaim) bool {
	n, e := s.queries.RenewProcessingLease(ctx, dbsql.RenewProcessingLeaseParams{ID: uuid(c.Run.ID), LeaseOwner: uuid(c.Owner), Fence: c.Fence, Revision: c.Run.Revision})
	return e == nil && n == 1
}
func (s *Store) ReleaseProcessing(ctx context.Context, c *ProcessingClaim) {
	_ = s.queries.ReleaseProcessingLease(ctx, dbsql.ReleaseProcessingLeaseParams{ID: uuid(c.Run.ID), LeaseOwner: uuid(c.Owner), Fence: c.Fence})
}
func (s *Store) ProcessingAuthority(ctx context.Context, r api.Run) error {
	return s.transaction(ctx, func(q *dbsql.Queries) error {
		return authorize(ctx, q, r.ProjectID, auth.Principal{Issuer: r.ActiveActor.Issuer, Subject: r.ActiveActor.Subject}, "process")
	})
}
func (s *Store) SaveProcessing(ctx context.Context, c *ProcessingClaim, r api.Run) error {
	return s.transaction(ctx, func(q *dbsql.Queries) error {
		b, e := q.LockProcessingRun(ctx, dbsql.LockProcessingRunParams{ProjectID: uuid(r.ProjectID), ID: uuid(r.ID)})
		if e != nil {
			return e
		}
		before, e := readRun(b)
		if e != nil {
			return e
		}
		valid, e := q.CheckProcessingLease(ctx, dbsql.CheckProcessingLeaseParams{ID: uuid(r.ID), LeaseOwner: uuid(c.Owner), Fence: c.Fence})
		if e != nil {
			return e
		}
		if !valid || before.Revision != c.Run.Revision {
			return ErrConflict
		}
		r.Revision = before.Revision + 1
		if !r.Valid() {
			return ErrProcessingUnavailable
		}
		n, e := q.UpdateProcessingRun(ctx, dbsql.UpdateProcessingRunParams{ID: uuid(r.ID), Revision: before.Revision, State: r.State, Snapshot: encoded(r)})
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrConflict
		}
		return saveProcessingHistory(ctx, q, before, r, "worker", nil)
	})
}
