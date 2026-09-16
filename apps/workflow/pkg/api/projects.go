package api

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	CodeBadRequest            = "BAD_REQUEST"
	CodeUnauthenticated       = "UNAUTHENTICATED"
	CodeDependencyUnavailable = "DEPENDENCY_UNAVAILABLE"
	DefaultProjectLimit       = 50
	MaxProjectLimit           = 100
)

type Project struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

type ProjectPage struct {
	Projects   []Project `json:"projects"`
	NextCursor *string   `json:"nextCursor"`
}

type ListProjectsOptions struct {
	Limit  int
	Cursor string
}

func (o ListProjectsOptions) Normalize() (ListProjectsOptions, error) {
	if o.Limit == 0 {
		o.Limit = DefaultProjectLimit
	}
	o.Cursor = strings.ToLower(o.Cursor)
	if o.Limit < 1 || o.Limit > MaxProjectLimit || (o.Cursor != "" && !ValidProjectID(o.Cursor)) {
		return ListProjectsOptions{}, errors.New("invalid project-list options")
	}
	return o, nil
}

// ValidProjectID accepts the canonical UUID representation emitted by PostgreSQL.
func ValidProjectID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// ValidFor bounds and checks the page, including ordering and forward-only cursors.
func (p ProjectPage) ValidFor(o ListProjectsOptions) bool {
	o, err := o.Normalize()
	if err != nil || p.Projects == nil || len(p.Projects) > o.Limit {
		return false
	}
	last := o.Cursor
	for _, project := range p.Projects {
		if !ValidProjectID(project.ID) || project.ID <= last || !utf8.ValidString(project.Name) || utf8.RuneCountInString(project.Name) > 200 || strings.Trim(project.Name, " ") == "" || len(project.Roles) == 0 || len(project.Roles) > 4 {
			return false
		}
		previous := -1
		for _, role := range project.Roles {
			rank := -1
			switch role {
			case "capture":
				rank = 0
			case "process":
				rank = 1
			case "review":
				rank = 2
			case "enroll":
				rank = 3
			}
			if rank <= previous {
				return false
			}
			previous = rank
		}
		last = project.ID
	}
	return p.NextCursor == nil || (len(p.Projects) == o.Limit && *p.NextCursor == last)
}
