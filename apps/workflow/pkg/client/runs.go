package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

const maxRunResponseBytes = 32 << 20

var errRunContract = errors.New("workflow response does not match the processing contract")

func runPath(project, id string) (string, error) {
	if !api.ValidID(project) || (id != "" && !api.ValidID(id)) {
		return "", errors.New("invalid Workflow resource ID")
	}
	p := "/pipeline/v1/projects/" + project + "/runs"
	if id != "" {
		p += "/" + id
	}
	return p, nil
}
func validToken(token string) bool {
	if len(token) < 1 || len(token) > 8<<10 {
		return false
	}
	for _, b := range []byte(token) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '.') {
			return false
		}
	}
	return true
}
func processingAPIError(body []byte, media string, status int) error {
	code := "HTTP_ERROR"
	expected := map[int]string{400: api.CodeBadRequest, 401: api.CodeUnauthenticated, 403: "FORBIDDEN", 404: "NOT_FOUND", 409: "CONFLICT", 503: api.CodeDependencyUnavailable}[status]
	var problem api.ErrorResponse
	if expected != "" && media == "application/json" && decodeJSON(body, &problem) == nil && problem.Error.Code == expected {
		code = expected
	}
	return &APIError{StatusCode: status, Code: code}
}
func (c *Client) processingRequest(ctx context.Context, token, method, path string, v any, target any) error {
	if !validToken(token) {
		return errors.New("invalid Workflow bearer token format")
	}
	var payload []byte
	var e error
	if v != nil {
		payload, e = json.Marshal(v)
		if e != nil || len(payload) > 4<<20 {
			return errors.New("invalid Workflow command")
		}
	}
	body, media, status, e := c.request(ctx, method, path, token, payload, maxRunResponseBytes)
	if e != nil {
		return e
	}
	if status != 200 {
		return processingAPIError(body, media, status)
	}
	if media != "application/json" || decodeJSON(body, target) != nil {
		return errRunContract
	}
	return nil
}
func (c *Client) runRequest(ctx context.Context, token, method, project, id, suffix string, v any) (r api.Run, e error) {
	p, e := runPath(project, id)
	if e != nil {
		return r, e
	}
	e = c.processingRequest(ctx, token, method, p+suffix, v, &r)
	if e != nil {
		return api.Run{}, e
	}
	if !r.Valid() || r.ProjectID != project || (id != "" && r.ID != id) {
		return api.Run{}, errRunContract
	}
	return
}
func (c *Client) RegisterRun(ctx context.Context, token, project string, v api.RegisterRun) (api.Run, error) {
	if !v.Valid() {
		return api.Run{}, errors.New("invalid run registration")
	}
	r, e := c.runRequest(ctx, token, "POST", project, "", "", v)
	if e == nil && api.Digest(r.Spec) != api.Digest(v) {
		return api.Run{}, errRunContract
	}
	return r, e
}
func (c *Client) GetRun(ctx context.Context, token, project, id string) (api.Run, error) {
	return c.runRequest(ctx, token, "GET", project, id, "", nil)
}
func (c *Client) command(ctx context.Context, token, project, id, action string, v api.RunCommand) (api.Run, error) {
	if !v.Valid() {
		return api.Run{}, errors.New("invalid run command")
	}
	return c.runRequest(ctx, token, http.MethodPost, project, id, "/"+action, v)
}
func (c *Client) VerifyInput(ctx context.Context, token, project, id string, v api.RunCommand) (api.Run, error) {
	return c.command(ctx, token, project, id, "verify", v)
}
func (c *Client) StartProcessing(ctx context.Context, token, project, id string, v api.RunCommand) (api.Run, error) {
	return c.command(ctx, token, project, id, "start", v)
}
func (c *Client) CancelProcessing(ctx context.Context, token, project, id string, v api.RunCommand) (api.Run, error) {
	return c.command(ctx, token, project, id, "cancel", v)
}
func (c *Client) RunEvents(ctx context.Context, token, project, id string, after int64) (page api.RunEventPage, e error) {
	p, e := runPath(project, id)
	if e != nil {
		return page, e
	}
	if after < 0 {
		return page, errors.New("invalid event cursor")
	}
	e = c.processingRequest(ctx, token, "GET", p+"/events?after="+strconv.FormatInt(after, 10), nil, &page)
	if e != nil {
		return api.RunEventPage{}, e
	}
	if page.Events == nil || len(page.Events) > 100 || page.NextCursor < 0 {
		return api.RunEventPage{}, errRunContract
	}
	last := after
	for _, v := range page.Events {
		if v.Sequence <= last || v.RunID != id || v.Revision < 1 || v.CreatedAt.IsZero() {
			return api.RunEventPage{}, errRunContract
		}
		last = v.Sequence
	}
	if page.NextCursor != 0 && (len(page.Events) != 100 || page.NextCursor != last) {
		return api.RunEventPage{}, errRunContract
	}
	return
}
func (c *Client) ReviewProcessing(ctx context.Context, token, project, id string, v api.ReviewRun) (api.Run, error) {
	if !v.RunCommand.Valid() || !api.ValidDigest(v.ResultDigest) || len(v.Decisions) < 1 || len(v.Decisions) > api.MaxLabels {
		return api.Run{}, errors.New("invalid processing review")
	}
	return c.runRequest(ctx, token, http.MethodPost, project, id, "/review", v)
}
func (c *Client) ApproveProcessing(ctx context.Context, token, project, id string, v api.ApproveRun) (api.Run, error) {
	if !v.RunCommand.Valid() || !api.ValidDigest(v.ResultDigest) || !api.ValidID(v.ReviewID) {
		return api.Run{}, errors.New("invalid processing approval")
	}
	return c.runRequest(ctx, token, http.MethodPost, project, id, "/approve", v)
}
func (c *Client) GetProcessingResult(ctx context.Context, token, project, id, attempt string) (r api.ProcessingResult, e error) {
	p, e := runPath(project, id)
	if e != nil {
		return r, e
	}
	if !api.ValidID(attempt) {
		return r, errors.New("invalid attempt ID")
	}
	e = c.processingRequest(ctx, token, "GET", p+"/results/"+attempt, nil, &r)
	if e == nil && (r.AttemptID != attempt || !api.ValidDigest(r.Digest) || r.Digest != r.SealedDigest() || len(r.Labels) < 1 || len(r.Labels) > api.MaxLabels) {
		return api.ProcessingResult{}, errRunContract
	}
	return
}
func (c *Client) GetProcessingArtifact(ctx context.Context, token, project, id, attempt, hash string) ([]byte, error) {
	p, e := runPath(project, id)
	if e != nil {
		return nil, e
	}
	if !api.ValidID(attempt) || !api.ValidDigest(hash) || !validToken(token) {
		return nil, errors.New("invalid artifact request")
	}
	b, media, status, e := c.get(ctx, p+"/results/"+attempt+"/artifacts/"+hash, token, int(api.MaxFileBytes))
	if e != nil {
		return nil, e
	}
	if status != 200 {
		return nil, processingAPIError(b, media, status)
	}
	if media != "image/png" || api.Hash(b) != hash {
		return nil, errRunContract
	}
	return b, nil
}

func (c *Client) GetApproval(ctx context.Context, token, project, id, approval string) (v api.ProcessingApproval, e error) {
	p, e := runPath(project, id)
	if e != nil {
		return v, e
	}
	if !api.ValidID(approval) {
		return v, errors.New("invalid approval ID")
	}
	e = c.processingRequest(ctx, token, "GET", fmt.Sprintf("%s/approvals/%s", p, approval), nil, &v)
	if e == nil && (v.ID != approval || v.Scope != "processing-result" || !api.ValidDigest(v.ResultDigest) || !api.ValidDigest(v.InputDigest) || !api.ValidDigest(v.ProfileDigest) || !api.ValidDigest(v.ReferenceDigest) || !api.ValidID(v.AttemptID) || !api.ValidID(v.ReviewID) || v.ApprovedBy.Issuer == "" || v.ApprovedBy.Subject == "" || v.CreatedAt.IsZero()) {
		return api.ProcessingApproval{}, errRunContract
	}
	return
}
