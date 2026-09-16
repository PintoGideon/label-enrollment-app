package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Processing approval is not authorization for APID extraction or enrollment.
const (
	MaxInputFiles       = 10000
	MaxInputBytes int64 = 16 << 30
	MaxFileBytes  int64 = 16 << 20
	MaxLabels           = 10000
	MaxAttempts         = 20
)

var component = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidDigest(s string) bool    { return digestPattern.MatchString(s) }
func ValidComponent(s string) bool { return component.MatchString(s) && s != "." && s != ".." }
func ValidID(s string) bool        { return ValidProjectID(s) && s == strings.ToLower(s) }
func Digest(v any) string          { b, _ := json.Marshal(v); return Hash(b) }
func Hash(b []byte) string         { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type Actor struct {
	Issuer  string `json:"issuer"`
	Subject string `json:"subject"`
}
type InputObject struct {
	Name   string `json:"name"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type RegisterRun struct {
	CommandID       string        `json:"commandId"`
	SourceID        string        `json:"sourceId"`
	Prefix          string        `json:"prefix"`
	ProfileID       string        `json:"profileId"`
	Objects         []InputObject `json:"objects"`
	ExpectedSerials []string      `json:"expectedSerials"`
}

func (v RegisterRun) Valid() bool {
	if !ValidID(v.CommandID) || !ValidComponent(v.SourceID) || !ValidComponent(v.ProfileID) || len(v.Prefix) > 512 || !strings.HasSuffix(v.Prefix, "/") || strings.HasPrefix(v.Prefix, "/") || len(v.Objects) < 1 || len(v.Objects) > MaxInputFiles || len(v.ExpectedSerials) > MaxLabels {
		return false
	}
	for _, part := range strings.Split(strings.TrimSuffix(v.Prefix, "/"), "/") {
		if !ValidComponent(part) {
			return false
		}
	}
	var size int64
	last := ""
	names := map[string]bool{}
	for _, o := range v.Objects {
		ext := strings.ToLower(path.Ext(o.Name))
		if !ValidComponent(o.Name) || names[strings.ToLower(o.Name)] || o.Name <= last || !slices.Contains([]string{".png", ".jpg", ".jpeg"}, ext) || o.Bytes < 1 || o.Bytes > MaxFileBytes || !ValidDigest(o.SHA256) {
			return false
		}
		last = o.Name
		names[strings.ToLower(o.Name)] = true
		size += o.Bytes
	}
	if size > MaxInputBytes {
		return false
	}
	seen := map[string]bool{}
	for _, s := range v.ExpectedSerials {
		if !ValidComponent(s) || seen[s] {
			return false
		}
		seen[s] = true
	}
	return true
}

type RunCommand struct {
	CommandID string `json:"commandId"`
	Revision  int64  `json:"revision"`
}

func (v RunCommand) Valid() bool { return ValidID(v.CommandID) && v.Revision > 0 }

type ReviewDecision struct {
	Serial string `json:"serial"`
	Accept bool   `json:"accept"`
}
type ReviewRun struct {
	RunCommand
	ResultDigest         string           `json:"resultDigest"`
	Decisions            []ReviewDecision `json:"decisions"`
	WarningsAcknowledged bool             `json:"warningsAcknowledged"`
}
type ApproveRun struct {
	RunCommand
	ResultDigest string `json:"resultDigest"`
	ReviewID     string `json:"reviewId"`
}
type VerifiedObject struct {
	InputObject
	Key       string `json:"key"`
	VersionID string `json:"versionId"`
	ETag      string `json:"etag"`
}
type VerifiedInput struct {
	Digest         string           `json:"digest"`
	Endpoint       string           `json:"endpoint"`
	Bucket         string           `json:"bucket"`
	Objects        []VerifiedObject `json:"objects"`
	CaptureHistory string           `json:"captureHistory"` // Existing-S3 imports have unknown capture history.
}

func (v VerifiedInput) SealedDigest() string { v.Digest = ""; return Digest(v) }

type Progress struct {
	Stage      string `json:"stage"`
	Frames     int    `json:"frames"`
	Boundaries int    `json:"boundaries"`
	Labels     int    `json:"labels"`
	Crops      int    `json:"crops"`
}
type Attempt struct {
	ID          string    `json:"id"`
	Number      int       `json:"number"`
	InitiatedBy Actor     `json:"initiatedBy"`
	Executor    string    `json:"executor"`
	Image       string    `json:"image"`
	CreatedAt   time.Time `json:"createdAt"`
	ExitCode    *int      `json:"exitCode"`
}
type Crop struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
type ResultLabel struct {
	Serial string          `json:"serial"`
	QR     string          `json:"qr"`
	Crops  map[string]Crop `json:"crops"`
}
type ProcessingResult struct {
	Digest          string        `json:"digest"`
	AttemptID       string        `json:"attemptId"`
	InputDigest     string        `json:"inputDigest"`
	ProfileDigest   string        `json:"profileDigest"`
	ReferenceDigest string        `json:"referenceDigest"`
	Image           string        `json:"image"`
	Labels          []ResultLabel `json:"labels"`
	Warnings        []string      `json:"warnings"`
}

// SealedDigest excludes only the self-referential digest field.
func (r ProcessingResult) SealedDigest() string { r.Digest = ""; return Digest(r) }

type ProcessingReview struct {
	ID                   string           `json:"id"`
	ResultDigest         string           `json:"resultDigest"`
	ReviewedBy           Actor            `json:"reviewedBy"`
	Decisions            []ReviewDecision `json:"decisions"`
	WarningsAcknowledged bool             `json:"warningsAcknowledged"`
	CreatedAt            time.Time        `json:"createdAt"`
}
type ProcessingApproval struct {
	ID              string    `json:"id"`
	Scope           string    `json:"scope"`
	ResultDigest    string    `json:"resultDigest"`
	InputDigest     string    `json:"inputDigest"`
	ProfileDigest   string    `json:"profileDigest"`
	ReferenceDigest string    `json:"referenceDigest"`
	AttemptID       string    `json:"attemptId"`
	ReviewID        string    `json:"reviewId"`
	ApprovedBy      Actor     `json:"approvedBy"`
	CreatedAt       time.Time `json:"createdAt"`
}
type Run struct {
	ID            string              `json:"id"`
	ProjectID     string              `json:"projectId"`
	Revision      int64               `json:"revision"`
	State         string              `json:"state"`
	Owner         Actor               `json:"owner"`
	RegisteredAt  time.Time           `json:"registeredAt"`
	Spec          RegisterRun         `json:"spec"`
	ProfileDigest string              `json:"profileDigest"`
	Input         *VerifiedInput      `json:"input"`
	Attempt       *Attempt            `json:"attempt"`
	Progress      Progress            `json:"progress"`
	Result        *ProcessingResult   `json:"result"`
	Review        *ProcessingReview   `json:"review"`
	Approval      *ProcessingApproval `json:"approval"`
	ApprovalIDs   []string            `json:"approvalIds"`
	ActiveActor   Actor               `json:"activeActor"`
	FailureCode   string              `json:"failureCode"`
}

func (r Run) Active() bool {
	return slices.Contains([]string{"verifying", "queued", "running", "validating", "cancelling", "uncertain"}, r.State)
}
func (r Run) Valid() bool {
	if !ValidID(r.ID) || !ValidID(r.ProjectID) || r.Revision < 1 || !r.Spec.Valid() || !ValidDigest(r.ProfileDigest) || r.Owner.Issuer == "" || r.Owner.Subject == "" || r.RegisteredAt.IsZero() || r.ApprovalIDs == nil || len(r.ApprovalIDs) > MaxAttempts {
		return false
	}
	if !slices.Contains([]string{"registered", "verifying", "verified", "queued", "running", "validating", "cancelling", "uncertain", "cancelled", "failed", "reviewable", "approved"}, r.State) {
		return false
	}
	if r.Input != nil && (!ValidDigest(r.Input.Digest) || len(r.Input.Objects) != len(r.Spec.Objects) || r.Input.CaptureHistory != "unknown") {
		return false
	}
	if r.Attempt != nil && (!ValidID(r.Attempt.ID) || r.Attempt.Number < 1 || r.Attempt.Number > MaxAttempts || r.Attempt.Executor != "local-docker" || !ValidDigest(strings.TrimPrefix(r.Attempt.Image, "sha256:"))) {
		return false
	}
	if r.Result != nil {
		if r.Attempt == nil || r.Input == nil || r.Result.AttemptID != r.Attempt.ID || r.Result.ProfileDigest != r.ProfileDigest || r.Result.InputDigest != r.Input.Digest || r.Result.Digest != r.Result.SealedDigest() || len(r.Result.Labels) != len(r.Spec.ExpectedSerials) {
			return false
		}
		for i, l := range r.Result.Labels {
			if l.Serial != r.Spec.ExpectedSerials[i] || len(l.Crops) < 1 || len(l.Crops) > 8 || len(l.QR) < 1 || len(l.QR) > 2048 {
				return false
			}
			for _, c := range l.Crops {
				if !ValidDigest(c.SHA256) || c.Bytes < 1 || c.Bytes > MaxFileBytes || c.Width < 1 || c.Height < 1 || c.Width > 4096 || c.Height > 4096 {
					return false
				}
			}
		}
	}
	if (r.State == "reviewable" || r.State == "approved") && r.Result == nil {
		return false
	}
	if r.Review != nil && (r.Result == nil || r.Review.ResultDigest != r.Result.Digest || !ValidID(r.Review.ID) || len(r.Review.Decisions) != len(r.Result.Labels)) {
		return false
	}
	if r.Approval != nil && (r.State != "approved" || r.Result == nil || r.Review == nil || r.Approval.Scope != "processing-result" || r.Approval.ResultDigest != r.Result.Digest || r.Approval.ReviewID != r.Review.ID) {
		return false
	}
	return r.State != "approved" || r.Approval != nil
}

type RunPage struct {
	Runs       []Run   `json:"runs"`
	NextCursor *string `json:"nextCursor"`
}
type RunEvent struct {
	Sequence  int64     `json:"sequence"`
	RunID     string    `json:"runId"`
	Revision  int64     `json:"revision"`
	Kind      string    `json:"kind"`
	Actor     *Actor    `json:"actor"`
	State     string    `json:"state"`
	Progress  Progress  `json:"progress"`
	CreatedAt time.Time `json:"createdAt"`
}
type RunEventPage struct {
	Events     []RunEvent `json:"events"`
	NextCursor int64      `json:"nextCursor"`
}
