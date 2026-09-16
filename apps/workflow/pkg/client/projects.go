package client

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

// Covers 100 projects with 200-character names even with JSON escaping.
// Probe limits remain 64 KiB; project responses are bounded separately.
const maxProjectResponseBytes = 256 << 10

func (c *Client) ListProjects(ctx context.Context, token string, options api.ListProjectsOptions) (api.ProjectPage, error) {
	options, err := options.Normalize()
	if err != nil {
		return api.ProjectPage{}, err
	}
	if len(token) == 0 || len(token) > 8<<10 {
		return api.ProjectPage{}, errors.New("a Workflow bearer token is required")
	}
	for _, b := range []byte(token) {
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '.') {
			return api.ProjectPage{}, errors.New("invalid Workflow bearer token format")
		}
	}
	query := url.Values{"limit": {strconv.Itoa(options.Limit)}}
	if options.Cursor != "" {
		query.Set("cursor", options.Cursor)
	}
	body, mediaType, status, err := c.get(ctx, "/pipeline/v1/projects?"+query.Encode(), token, maxProjectResponseBytes)
	if err != nil {
		return api.ProjectPage{}, err
	}
	if status != http.StatusOK {
		code := "HTTP_ERROR"
		var problem api.ErrorResponse
		if mediaType == "application/json" && decodeJSON(body, &problem) == nil {
			expected := map[int]string{
				http.StatusBadRequest:         api.CodeBadRequest,
				http.StatusUnauthorized:       api.CodeUnauthenticated,
				http.StatusServiceUnavailable: api.CodeDependencyUnavailable,
			}[status]
			if expected != "" && problem.Error.Code == expected {
				code = expected
			}
		}
		return api.ProjectPage{}, &APIError{StatusCode: status, Code: code}
	}
	// Pointers distinguish a missing nextCursor from the required explicit null.
	var wire struct {
		Projects   *[]api.Project `json:"projects"`
		NextCursor jsonCursor     `json:"nextCursor"`
	}
	if mediaType != "application/json" || decodeJSON(body, &wire) != nil || wire.Projects == nil || !wire.NextCursor.present {
		return api.ProjectPage{}, errors.New("workflow response does not match the project-list contract")
	}
	page := api.ProjectPage{Projects: *wire.Projects, NextCursor: wire.NextCursor.value}
	if !page.ValidFor(options) {
		return api.ProjectPage{}, errors.New("workflow response does not match the project-list contract")
	}
	return page, nil
}

type jsonCursor struct {
	present bool
	value   *string
}

func (c *jsonCursor) UnmarshalJSON(data []byte) error {
	c.present = true
	return decodeJSON(data, &c.value)
}
