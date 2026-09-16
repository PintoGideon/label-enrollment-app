package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/auth"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

type TokenVerifier interface {
	Verify(context.Context, string) (auth.Principal, error)
}

type ProjectLister interface {
	ListProjects(context.Context, auth.Principal, api.ListProjectsOptions) (api.ProjectPage, error)
}

func listProjects(verifier TokenVerifier, projects ProjectLister) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2500*time.Millisecond)
		defer cancel()
		values := r.Header.Values("Authorization")
		if len(values) != 1 {
			projectError(w, http.StatusUnauthorized)
			return
		}
		scheme, token, ok := strings.Cut(values[0], " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || len(token) > auth.MaxTokenBytes || strings.ContainsAny(token, " \t\r\n,") {
			projectError(w, http.StatusUnauthorized)
			return
		}
		if verifier == nil {
			projectError(w, http.StatusServiceUnavailable)
			return
		}
		principal, err := verifier.Verify(ctx, token)
		if err != nil || !principal.Valid() {
			status := http.StatusServiceUnavailable
			if errors.Is(err, auth.ErrInvalid) || err == nil {
				status = http.StatusUnauthorized
			}
			projectError(w, status)
			return
		}
		// Authentication precedes parameter parsing. Claimed org/actor headers
		// are intentionally not used as project membership or identity.
		options, err := projectOptions(r.URL.RawQuery)
		if err != nil {
			projectError(w, http.StatusBadRequest)
			return
		}
		if projects == nil {
			projectError(w, http.StatusServiceUnavailable)
			return
		}
		page, err := projects.ListProjects(ctx, principal, options)
		if err != nil || !page.ValidFor(options) {
			projectError(w, http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusOK, page)
	}
}

func projectOptions(raw string) (api.ListProjectsOptions, error) {
	bad := errors.New("invalid project-list parameters")
	if len(raw) > 256 {
		return api.ListProjectsOptions{}, bad
	}
	query, err := url.ParseQuery(raw)
	if err != nil {
		return api.ListProjectsOptions{}, bad
	}
	options := api.ListProjectsOptions{}
	for name, values := range query {
		if len(values) != 1 || values[0] == "" {
			return options, bad
		}
		switch name {
		case "limit":
			for _, c := range values[0] {
				if c < '0' || c > '9' {
					return options, bad
				}
			}
			options.Limit, err = strconv.Atoi(values[0])
			if err != nil || options.Limit < 1 {
				return options, bad
			}
		case "cursor":
			options.Cursor = values[0]
		default:
			return options, bad
		}
	}
	return options.Normalize()
}

func projectError(w http.ResponseWriter, status int) {
	detail := api.ErrorDetail{Code: api.CodeDependencyUnavailable, Message: "Workflow project access is temporarily unavailable."}
	switch status {
	case http.StatusUnauthorized:
		w.Header().Set("WWW-Authenticate", `Bearer realm="workflow"`)
		detail = api.ErrorDetail{Code: api.CodeUnauthenticated, Message: "A valid Workflow access token is required."}
	case http.StatusBadRequest:
		detail = api.ErrorDetail{Code: api.CodeBadRequest, Message: "Invalid project-list parameters."}
	}
	writeJSON(w, status, api.ErrorResponse{Error: detail})
}
