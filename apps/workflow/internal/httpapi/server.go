// Package httpapi owns HTTP routing and server lifecycle, not orchestration logic.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

type DatabaseChecker interface {
	Check(context.Context) api.DatabaseStatus
}

func NewHandler(database DatabaseChecker) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, api.ProbeResponse{Status: api.StatusOK, Service: api.ServiceName})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		status := api.DatabaseNotConfigured
		if database != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			status = database.Check(ctx)
			if !status.Valid() {
				status = api.DatabaseUnavailable
			}
		}
		// Database readiness does not make an unimplemented authorized API ready.
		writeJSON(w, http.StatusServiceUnavailable, api.ErrorResponse{
			Error: api.ErrorDetail{
				Code: api.CodeNotReady, Message: "Authenticated project API is not implemented yet.",
			},
			Checks: &api.ReadinessChecks{Database: status, ProjectAPI: api.ProjectAPINotImplemented},
		})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Only fixed, marshalable contract structs are written here.
	_ = json.NewEncoder(w).Encode(value)
}

func NewServer(database DatabaseChecker) *http.Server {
	return &http.Server{
		Handler:           NewHandler(database),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
}

// Serve owns listener after invocation. Cancellation drains requests for up to
// five seconds, then closes remaining connections rather than orphaning a server.
func Serve(ctx context.Context, listener net.Listener, database DatabaseChecker) error {
	server := NewServer(database)
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()

	select {
	case err := <-finished:
		return serveError(err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			<-finished
			return errors.New("workflow shutdown did not complete gracefully")
		}
		return serveError(<-finished)
	}
}

func serveError(err error) error {
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
