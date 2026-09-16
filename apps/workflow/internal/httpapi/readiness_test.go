package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

type checkFunc func(context.Context) api.DatabaseStatus

func (f checkFunc) Check(ctx context.Context) api.DatabaseStatus { return f(ctx) }

func TestDatabaseChecksDoNotGrantProjectAPIReadiness(t *testing.T) {
	for _, state := range []api.DatabaseStatus{api.DatabaseReady, api.DatabaseMigrationRequired, api.DatabaseSchemaMismatch, api.DatabaseUnavailable, "unknown"} {
		t.Run(string(state), func(t *testing.T) {
			handler := NewHandler(checkFunc(func(ctx context.Context) api.DatabaseStatus {
				if _, bounded := ctx.Deadline(); !bounded {
					t.Error("dependency check lacks deadline")
				}
				return state
			}), nil, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			var result api.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			want := state
			if !want.Valid() {
				want = api.DatabaseUnavailable
			}
			if response.Code != http.StatusServiceUnavailable || result.Checks == nil || result.Checks.Database != want || result.Checks.ProjectAPI != api.ProjectAPINotImplemented {
				t.Fatal("incorrect checks or false overall readiness")
			}
		})
	}
}
