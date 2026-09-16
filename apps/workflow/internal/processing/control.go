package processing

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/auth"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/config"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/database"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

type Service struct {
	Store    *database.Store
	cfg      config.Processing
	profiles map[string]profile
}

func New(store *database.Store, cfg *config.Processing) (*Service, error) {
	if cfg == nil {
		return nil, nil
	}
	if store == nil {
		return nil, database.ErrProcessingUnavailable
	}
	if e := os.MkdirAll(cfg.Root, 0700); e != nil {
		return nil, database.ErrProcessingUnavailable
	}
	resolved, e := filepath.EvalSymlinks(cfg.Root)
	if e != nil || resolved != cfg.Root {
		return nil, database.ErrProcessingUnavailable
	}
	s := &Service{Store: store, cfg: *cfg, profiles: map[string]profile{}}
	for _, p := range cfg.Profiles {
		f, e := loadProfile(cfg.Root, p)
		if e != nil {
			return nil, database.ErrProcessingUnavailable
		}
		s.profiles[p.ID] = f
	}
	return s, nil
}
func actor(p auth.Principal) api.Actor { return api.Actor{Issuer: p.Issuer, Subject: p.Subject} }
func (s *Service) source(project string, spec api.RegisterRun) (config.ProcessingSource, bool) {
	for _, v := range s.cfg.Sources {
		if v.ID == spec.SourceID && v.ProjectID == project && strings.HasPrefix(spec.Prefix, v.Prefix) && slices.Contains(v.Profiles, spec.ProfileID) {
			return v, true
		}
	}
	return config.ProcessingSource{}, false
}
func (s *Service) Register(ctx context.Context, p auth.Principal, project string, v api.RegisterRun) (api.Run, error) {
	if !api.ValidID(project) || !v.Valid() {
		return api.Run{}, database.ErrBadCommand
	}
	return s.Store.ApplyProcessingCommand(ctx, p, project, "", v.CommandID, "capture", "register", v, func(r *api.Run) error {
		if _, ok := s.source(project, v); !ok {
			return database.ErrForbidden
		}
		profile, ok := s.profiles[v.ProfileID]
		if !ok {
			return database.ErrForbidden
		}
		if (profile.Policy.Purpose == "synthetic" && len(v.ExpectedSerials) == 0) || (profile.Policy.Purpose == "inspection" && len(v.ExpectedSerials) != 0) {
			return database.ErrBadCommand
		}
		for _, serial := range v.ExpectedSerials {
			if profile.References[serial] == "" {
				return database.ErrBadCommand
			}
		}
		r.Owner = actor(p)
		r.ActiveActor = r.Owner
		r.Spec = v
		r.State = "registered"
		r.RegisteredAt = time.Now().UTC()
		r.ProfileDigest = profile.Digest
		r.Progress.Stage = "registered"
		return nil
	})
}
func (s *Service) Command(ctx context.Context, p auth.Principal, project, id, action string, v api.RunCommand) (api.Run, error) {
	if !api.ValidID(project) || !api.ValidID(id) || !v.Valid() || !slices.Contains([]string{"verify", "start", "cancel"}, action) {
		return api.Run{}, database.ErrBadCommand
	}
	return s.Store.ApplyProcessingCommand(ctx, p, project, id, v.CommandID, "process", action, v, func(r *api.Run) error {
		if r.Revision != v.Revision {
			return database.ErrConflict
		}
		switch action {
		case "verify":
			if r.Input != nil || r.Active() || (r.State != "registered" && r.State != "failed" && r.State != "cancelled") {
				return database.ErrConflict
			}
			r.State = "verifying"
			r.Progress = api.Progress{Stage: "verifying_input"}
		case "start":
			if r.Input == nil || r.Active() {
				return database.ErrConflict
			}
			profile, ok := s.profiles[r.Spec.ProfileID]
			if !ok || profile.Digest != r.ProfileDigest {
				return database.ErrConflict
			}
			if _, ok = s.source(project, r.Spec); !ok {
				return database.ErrForbidden
			}
			number := 1
			if r.Attempt != nil {
				number = r.Attempt.Number + 1
			}
			if number > api.MaxAttempts {
				return database.ErrConflict
			}
			r.Attempt = &api.Attempt{ID: database.NewID(), Number: number, InitiatedBy: actor(p), Executor: "local-docker", Image: s.cfg.Image, CreatedAt: time.Now().UTC()}
			r.Result = nil
			r.Review = nil
			r.Approval = nil
			r.State = "queued"
			r.Progress = api.Progress{Stage: "queued"}
		case "cancel":
			if !r.Active() {
				return database.ErrConflict
			}
			r.State = "cancelling"
		}
		r.ActiveActor = actor(p)
		r.FailureCode = ""
		return nil
	})
}
func (s *Service) ReadRun(ctx context.Context, p auth.Principal, project, id string) (api.Run, error) {
	return s.Store.ReadRun(ctx, p, project, id)
}
func (s *Service) Events(ctx context.Context, p auth.Principal, project, id string, after int64) (api.RunEventPage, error) {
	return s.Store.ReadRunEvents(ctx, p, project, id, after)
}
func (s *Service) Approval(ctx context.Context, p auth.Principal, project, id, approval string) (api.ProcessingApproval, error) {
	return s.Store.ReadApproval(ctx, p, project, id, approval)
}
