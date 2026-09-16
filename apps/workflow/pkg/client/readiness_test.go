package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

func TestTypedReadinessChecks(t *testing.T) {
	for _, state := range []string{"ready", "migration_required", "schema_mismatch", "unavailable", "invalid"} {
		t.Run(state, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintf(w, `{"error":{"code":"NOT_READY","message":"not ready"},"checks":{"database":%q,"projectApi":"not_implemented"}}`, state)
			}))
			defer server.Close()
			c, err := New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.Readiness(context.Background())
			var problem *APIError
			if state == "invalid" {
				if err == nil || errors.As(err, &problem) {
					t.Fatal("invalid checks must fail contract validation")
				}
				return
			}
			if !errors.As(err, &problem) || problem.Checks == nil || problem.Checks.Database != api.DatabaseStatus(state) || problem.Checks.ProjectAPI != api.ProjectAPINotImplemented {
				t.Fatal("typed readiness details missing")
			}
		})
	}
}
