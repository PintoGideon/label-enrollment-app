package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/auth"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/database"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

type ProcessingAPI interface {
	Register(context.Context, auth.Principal, string, api.RegisterRun) (api.Run, error)
	Command(context.Context, auth.Principal, string, string, string, api.RunCommand) (api.Run, error)
	ReadRun(context.Context, auth.Principal, string, string) (api.Run, error)
	Events(context.Context, auth.Principal, string, string, int64) (api.RunEventPage, error)
	Approval(context.Context, auth.Principal, string, string, string) (api.ProcessingApproval, error)
	Review(context.Context, auth.Principal, string, string, api.ReviewRun) (api.Run, error)
	Approve(context.Context, auth.Principal, string, string, api.ApproveRun) (api.Run, error)
	Result(context.Context, auth.Principal, string, string, string) (api.ProcessingResult, error)
	Artifact(context.Context, auth.Principal, string, string, string, string) ([]byte, error)
}

func processingRoutes(mux *http.ServeMux, verifier TokenVerifier, service ProcessingAPI) {
	base := "/pipeline/v1/projects/{project}/runs"
	handle := func(pattern, action string) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
			defer cancel()
			p, status := processingPrincipal(ctx, r, verifier)
			if status != 0 {
				processingError(w, status)
				return
			}
			project, id := r.PathValue("project"), r.PathValue("run")
			if !api.ValidID(project) || (id != "" && !api.ValidID(id)) {
				processingError(w, 400)
				return
			}
			if service == nil {
				processingError(w, 503)
				return
			}
			if action != "events" && r.URL.RawQuery != "" {
				processingError(w, 400)
				return
			}
			var value any
			var e error
			switch action {
			case "register":
				var v api.RegisterRun
				if !processingBody(w, r, &v) || !v.Valid() {
					processingError(w, 400)
					return
				}
				value, e = service.Register(ctx, p, project, v)
			case "read":
				value, e = service.ReadRun(ctx, p, project, id)
			case "events":
				after := int64(0)
				q, err := r.URL.Query(), error(nil)
				if len(r.URL.RawQuery) > 64 || len(q) > 1 || len(q["after"]) > 1 {
					processingError(w, 400)
					return
				}
				if r.URL.RawQuery != "" {
					if len(q["after"]) != 1 {
						processingError(w, 400)
						return
					}
					after, err = strconv.ParseInt(q.Get("after"), 10, 64)
				}
				if err != nil || after < 0 {
					processingError(w, 400)
					return
				}
				value, e = service.Events(ctx, p, project, id, after)
			case "review":
				var v api.ReviewRun
				if !processingBody(w, r, &v) {
					processingError(w, 400)
					return
				}
				value, e = service.Review(ctx, p, project, id, v)
			case "approve":
				var v api.ApproveRun
				if !processingBody(w, r, &v) {
					processingError(w, 400)
					return
				}
				value, e = service.Approve(ctx, p, project, id, v)
			case "result", "artifact":
				attempt := r.PathValue("attempt")
				if !api.ValidID(attempt) {
					processingError(w, 400)
					return
				}
				if action == "result" {
					value, e = service.Result(ctx, p, project, id, attempt)
				} else {
					hash := r.PathValue("artifact")
					if !api.ValidDigest(hash) {
						processingError(w, 400)
						return
					}
					var data []byte
					data, e = service.Artifact(ctx, p, project, id, attempt, hash)
					if e == nil {
						w.Header().Set("Content-Type", "image/png")
						w.Header().Set("Content-Length", strconv.Itoa(len(data)))
						w.WriteHeader(200)
						_, _ = w.Write(data)
						return
					}
				}
			case "approval":
				a := r.PathValue("approval")
				if !api.ValidID(a) {
					processingError(w, 400)
					return
				}
				value, e = service.Approval(ctx, p, project, id, a)
			default:
				var v api.RunCommand
				if !processingBody(w, r, &v) || !v.Valid() {
					processingError(w, 400)
					return
				}
				value, e = service.Command(ctx, p, project, id, action, v)
			}
			if e != nil {
				processingFailure(w, e)
				return
			}
			writeJSON(w, 200, value)
		})
	}
	handle("POST "+base, "register")
	handle("GET "+base+"/{run}", "read")
	handle("GET "+base+"/{run}/events", "events")
	handle("GET "+base+"/{run}/approvals/{approval}", "approval")
	handle("GET "+base+"/{run}/results/{attempt}", "result")
	handle("GET "+base+"/{run}/results/{attempt}/artifacts/{artifact}", "artifact")
	for _, action := range []string{"verify", "start", "cancel", "review", "approve"} {
		handle("POST "+base+"/{run}/"+action, action)
	}
}
func processingPrincipal(ctx context.Context, r *http.Request, v TokenVerifier) (auth.Principal, int) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return auth.Principal{}, 401
	}
	scheme, token, ok := strings.Cut(values[0], " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || len(token) > auth.MaxTokenBytes || strings.ContainsAny(token, " \t\r\n,") {
		return auth.Principal{}, 401
	}
	if v == nil {
		return auth.Principal{}, 503
	}
	p, e := v.Verify(ctx, token)
	if e != nil || !p.Valid() {
		if errors.Is(e, auth.ErrInvalid) || e == nil {
			return auth.Principal{}, 401
		}
		return auth.Principal{}, 503
	}
	return p, 0
}
func processingBody(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "application/json" || r.Header.Get("Content-Encoding") != "" {
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20))
	d.DisallowUnknownFields()
	return d.Decode(v) == nil && d.Decode(new(any)) == io.EOF
}
func processingFailure(w http.ResponseWriter, e error) {
	status := 503
	switch {
	case errors.Is(e, database.ErrBadCommand):
		status = 400
	case errors.Is(e, database.ErrNotFound):
		status = 404
	case errors.Is(e, database.ErrForbidden):
		status = 403
	case errors.Is(e, database.ErrConflict):
		status = 409
	}
	processingError(w, status)
}
func processingError(w http.ResponseWriter, status int) {
	code, message := api.CodeDependencyUnavailable, "Workflow processing is temporarily unavailable."
	switch status {
	case 400:
		code, message = api.CodeBadRequest, "Invalid processing request."
	case 401:
		code, message = api.CodeUnauthenticated, "A valid Workflow access token is required."
		w.Header().Set("WWW-Authenticate", `Bearer realm="workflow"`)
	case 403:
		code, message = "FORBIDDEN", "This processing action is not permitted."
	case 404:
		code, message = "NOT_FOUND", "Processing resource not found."
	case 409:
		code, message = "CONFLICT", "Command conflicts with current state, revision or immutable content."
	}
	writeJSON(w, status, api.ErrorResponse{Error: api.ErrorDetail{Code: code, Message: message}})
}
