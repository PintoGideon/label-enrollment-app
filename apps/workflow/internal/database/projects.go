package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/auth"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/database/dbsql"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

var ErrProjectsUnavailable = errors.New("Workflow projects are unavailable")

func (s *Store) ListProjects(ctx context.Context, principal auth.Principal, options api.ListProjectsOptions) (api.ProjectPage, error) {
	options, err := options.Normalize()
	if err != nil || !principal.Valid() || s == nil {
		return api.ProjectPage{}, ErrProjectsUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if s.Check(ctx) != api.DatabaseReady {
		return api.ProjectPage{}, ErrProjectsUnavailable
	}
	var cursor pgtype.UUID
	if options.Cursor != "" {
		if err := cursor.Scan(options.Cursor); err != nil {
			return api.ProjectPage{}, ErrProjectsUnavailable
		}
	}
	rows, err := s.queries.ListAuthorizedProjects(ctx, dbsql.ListAuthorizedProjectsParams{
		Issuer: principal.Issuer, Subject: principal.Subject, Cursor: cursor, PageSize: int32(options.Limit + 1),
	})
	if err != nil {
		return api.ProjectPage{}, ErrProjectsUnavailable
	}
	page := api.ProjectPage{Projects: make([]api.Project, 0, min(len(rows), options.Limit))}
	more := len(rows) > options.Limit
	if more {
		rows = rows[:options.Limit]
	}
	for _, row := range rows {
		page.Projects = append(page.Projects, api.Project{ID: row.ID.String(), Name: row.Name, Roles: row.Roles})
	}
	if more {
		last := page.Projects[len(page.Projects)-1].ID
		page.NextCursor = &last
	}
	if !page.ValidFor(options) {
		return api.ProjectPage{}, ErrProjectsUnavailable
	}
	return page, nil
}
