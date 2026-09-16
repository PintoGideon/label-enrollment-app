package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/auth"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/database"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

var errResultInvalid = errors.New("processing result failed validation")
var errReferenceRequired = errors.New("inspection requires serial authority and expected associations before approval")

func decodeResult(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		return errResultInvalid
	}
	return nil
}

type reelDocument struct {
	Schema      int `json:"schemaVersion"`
	Identifiers map[string]struct {
		Type        string `json:"type"`
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"identifiers"`
	Labels []map[string]string `json:"labels"`
}
type summaryDocument struct {
	Schema int                 `json:"schemaVersion"`
	Labels []map[string]string `json:"labels"`
}

func (s *Service) importResult(ctx context.Context, r api.Run) (*api.ProcessingResult, error) {
	p, ok := s.profiles[r.Spec.ProfileID]
	if !ok || p.Digest != r.ProfileDigest || r.Attempt == nil || r.Attempt.ExitCode == nil || *r.Attempt.ExitCode != 0 || s.checkInput(ctx, r) != nil || s.checkProfile(p) != nil {
		return nil, errResultInvalid
	}
	root := s.outputDir(r)
	actual, e := readFile(root, "config.yaml", api.MaxFileBytes)
	if e != nil {
		return nil, errResultInvalid
	}
	expected, e := readFile(p.Dir, p.Policy.Config, api.MaxFileBytes)
	if e != nil || !bytes.Equal(actual, expected) {
		return nil, errResultInvalid
	}
	perf, e := s.readPerformance(r)
	if p.Policy.Purpose == "inspection" {
		// This deliberately cannot create an approvable result without serial
		// authority. The original engine outputs remain available for harness
		// inspection; neither QR-derived IDs nor expected counts are invented.
		if e != nil {
			return nil, errResultInvalid
		}
		if _, e = time.Parse(time.RFC3339, perf.Finished); e != nil {
			return nil, errResultInvalid
		}
		for _, name := range []string{"labels.json", "crops/reel.json"} {
			if _, e = readFile(root, name, 4<<20); e != nil {
				return nil, errResultInvalid
			}
		}
		return nil, errReferenceRequired
	}
	if e != nil || perf.Regroup.Labels != len(r.Spec.ExpectedSerials) || perf.Regroup.Complete != perf.Regroup.Labels || perf.Regroup.Failed != 0 || perf.Regroup.Crops < len(r.Spec.ExpectedSerials)*len(p.Policy.Slots) {
		return nil, errResultInvalid
	}
	if _, e = time.Parse(time.RFC3339, perf.Finished); e != nil {
		return nil, errResultInvalid
	}
	b, e := readFile(root, "crops/reel.json", 4<<20)
	if e != nil {
		return nil, errResultInvalid
	}
	var reel reelDocument
	if decodeResult(b, &reel) != nil || reel.Schema != 2 || len(reel.Labels) != len(r.Spec.ExpectedSerials) || len(reel.Identifiers) != len(p.Policy.Slots)+2 {
		return nil, errResultInvalid
	}
	if reel.Identifiers["serial"].Type != "text" || reel.Identifiers["qr"].Type != "qr" {
		return nil, errResultInvalid
	}
	for _, slot := range p.Policy.Slots {
		if reel.Identifiers[slot.Key].Type != "dust" {
			return nil, errResultInvalid
		}
	}
	b, e = readFile(root, "labels.json", 4<<20)
	if e != nil {
		return nil, errResultInvalid
	}
	var summary summaryDocument
	if decodeResult(b, &summary) != nil || summary.Schema != 1 || len(summary.Labels) != len(reel.Labels) {
		return nil, errResultInvalid
	}
	warnings, e := readFile(root, "warnings.txt", 64<<10)
	if e != nil {
		return nil, errResultInvalid
	}
	result := &api.ProcessingResult{AttemptID: r.Attempt.ID, InputDigest: r.Input.Digest, ProfileDigest: r.ProfileDigest, ReferenceDigest: p.ReferenceDigest, Image: r.Attempt.Image, Labels: []api.ResultLabel{}, Warnings: []string{}}
	// Preserve the bounded warning evidence, including the zero-warning header.
	// Never serve the engine's executable HTML report as an authenticated preview.
	for _, line := range strings.Split(strings.TrimSpace(string(warnings)), "\n") {
		if line != "" {
			result.Warnings = append(result.Warnings, line)
		}
	}
	paths := map[string]bool{}
	var total int64
	for i, row := range reel.Labels {
		serial := r.Spec.ExpectedSerials[i]
		qr := p.References[serial]
		if len(row) != len(reel.Identifiers) || row["serial"] != serial || row["qr"] != qr || qr == "" || summary.Labels[i]["serial-number"] != serial || summary.Labels[i]["decoded-qr"] != qr {
			return nil, errResultInvalid
		}
		label := api.ResultLabel{Serial: serial, QR: qr, Crops: map[string]api.Crop{}}
		for _, slot := range p.Policy.Slots {
			name := row[slot.Key]
			if !fs.ValidPath(name) || name == "." || !strings.HasSuffix(name, ".png") || paths[name] || summary.Labels[i][slot.SummaryKey] != name {
				return nil, errResultInvalid
			}
			paths[name] = true
			data, e := readFile(root, "crops/"+name, api.MaxFileBytes)
			if e != nil {
				return nil, errResultInvalid
			}
			total += int64(len(data))
			if total > api.MaxInputBytes {
				return nil, errResultInvalid
			}
			dimensions, e := png.DecodeConfig(bytes.NewReader(data))
			if e != nil || dimensions.Width != slot.Width || dimensions.Height != slot.Height {
				return nil, errResultInvalid
			}
			img, e := png.Decode(bytes.NewReader(data))
			if e != nil || img.Bounds().Dx() != slot.Width || img.Bounds().Dy() != slot.Height {
				return nil, errResultInvalid
			}
			hash := api.Hash(data)
			if e = s.publishArtifact(hash, data); e != nil {
				return nil, e
			}
			label.Crops[slot.Key] = api.Crop{SHA256: hash, Bytes: int64(len(data)), Width: slot.Width, Height: slot.Height}
		}
		result.Labels = append(result.Labels, label)
	}
	result.Digest = result.SealedDigest()
	return result, nil
}
func (s *Service) publishArtifact(hash string, b []byte) error {
	root := filepath.Join(s.cfg.Root, "artifacts")
	if e := os.MkdirAll(root, 0700); e != nil {
		return e
	}
	name := hash + ".png"
	path := filepath.Join(root, name)
	// Publish by exclusive link from a completely written/synced private file.
	// Readers never see a partially written artifact; an existing digest is never
	// overwritten, even if damaged. Failed DB commits leave harmless orphan bytes.
	f, e := os.CreateTemp(root, ".staging-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(b); e == nil {
		e = f.Chmod(0400)
	}
	if e == nil {
		e = f.Sync()
	}
	_ = f.Close()
	if e != nil {
		return e
	}
	if e = os.Link(tmp, path); e != nil && !errors.Is(e, os.ErrExist) {
		return e
	}
	existing, e := readFile(root, name, api.MaxFileBytes)
	if e != nil || api.Hash(existing) != hash || !bytes.Equal(existing, b) {
		return errResultInvalid
	}
	return nil
}
func (s *Service) sealedResult(r *api.ProcessingResult) error {
	if r == nil || r.Digest != r.SealedDigest() {
		return errResultInvalid
	}
	for _, label := range r.Labels {
		for _, crop := range label.Crops {
			b, e := readFile(filepath.Join(s.cfg.Root, "artifacts"), crop.SHA256+".png", api.MaxFileBytes)
			if e != nil || int64(len(b)) != crop.Bytes || api.Hash(b) != crop.SHA256 {
				return errResultInvalid
			}
		}
	}
	return nil
}
func (s *Service) Review(ctx context.Context, p auth.Principal, project, id string, v api.ReviewRun) (api.Run, error) {
	if !v.RunCommand.Valid() || !api.ValidDigest(v.ResultDigest) || len(v.Decisions) < 1 || len(v.Decisions) > api.MaxLabels {
		return api.Run{}, database.ErrBadCommand
	}
	return s.Store.ApplyProcessingCommand(ctx, p, project, id, v.CommandID, "review", "review", v, func(r *api.Run) error {
		if r.Revision != v.Revision || r.State != "reviewable" || r.Result == nil || r.Result.Digest != v.ResultDigest || len(v.Decisions) != len(r.Result.Labels) || s.sealedResult(r.Result) != nil {
			return database.ErrConflict
		}
		for i, d := range v.Decisions {
			if d.Serial != r.Result.Labels[i].Serial {
				return database.ErrBadCommand
			}
		}
		r.Review = &api.ProcessingReview{ID: database.NewID(), ResultDigest: v.ResultDigest, ReviewedBy: actor(p), Decisions: v.Decisions, WarningsAcknowledged: v.WarningsAcknowledged, CreatedAt: time.Now().UTC()}
		r.Progress.Stage = "reviewed"
		return nil
	})
}
func (s *Service) Approve(ctx context.Context, p auth.Principal, project, id string, v api.ApproveRun) (api.Run, error) {
	if !v.RunCommand.Valid() || !api.ValidDigest(v.ResultDigest) || !api.ValidID(v.ReviewID) {
		return api.Run{}, database.ErrBadCommand
	}
	return s.Store.ApplyProcessingCommand(ctx, p, project, id, v.CommandID, "review", "approve", v, func(r *api.Run) error {
		if r.Revision != v.Revision || r.State != "reviewable" || r.Result == nil || r.Review == nil || r.Result.Digest != v.ResultDigest || r.Review.ID != v.ReviewID || !r.Review.WarningsAcknowledged || s.sealedResult(r.Result) != nil {
			return database.ErrConflict
		}
		for _, d := range r.Review.Decisions {
			if !d.Accept {
				return database.ErrConflict
			}
		}
		r.Approval = &api.ProcessingApproval{ID: database.NewID(), Scope: "processing-result", ResultDigest: r.Result.Digest, InputDigest: r.Input.Digest, ProfileDigest: r.ProfileDigest, ReferenceDigest: r.Result.ReferenceDigest, AttemptID: r.Attempt.ID, ReviewID: r.Review.ID, ApprovedBy: actor(p), CreatedAt: time.Now().UTC()}
		r.ApprovalIDs = append(r.ApprovalIDs, r.Approval.ID)
		r.State = "approved"
		r.Progress.Stage = "approved"
		return nil
	})
}
func (s *Service) Result(ctx context.Context, p auth.Principal, project, id, attempt string) (api.ProcessingResult, error) {
	return s.Store.ReadResult(ctx, p, project, id, attempt)
}
func (s *Service) Artifact(ctx context.Context, p auth.Principal, project, id, attempt, hash string) ([]byte, error) {
	result, e := s.Store.ReadResult(ctx, p, project, id, attempt)
	if e != nil {
		return nil, e
	}
	for _, label := range result.Labels {
		for _, crop := range label.Crops {
			if crop.SHA256 == hash {
				b, e := readFile(filepath.Join(s.cfg.Root, "artifacts"), hash+".png", api.MaxFileBytes)
				if e != nil || int64(len(b)) != crop.Bytes || api.Hash(b) != hash {
					return nil, database.ErrProcessingUnavailable
				}
				return b, nil
			}
		}
	}
	return nil, database.ErrNotFound
}
