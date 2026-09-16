package httpapi

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

func TestProbeContracts(t *testing.T) {
	handler := NewHandler(nil, nil, nil)
	for _, path := range []string{"/healthz", "/readyz"} {
		t.Run(path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Content-Type") != "application/json" {
				t.Fatal("missing response safety/content headers")
			}
			if path == "/healthz" {
				var result api.ProbeResponse
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if response.Code != http.StatusOK || result.Status != api.StatusOK || result.Service != api.ServiceName {
					t.Fatalf("incorrect liveness: %d %+v", response.Code, result)
				}
			} else {
				var result api.ErrorResponse
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if response.Code != http.StatusServiceUnavailable || result.Error.Code != api.CodeNotReady {
					t.Fatal("unimplemented backend must not claim readiness")
				}
			}
		})
	}
}

func TestFoundationRouteBoundaries(t *testing.T) {
	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodPost, "/readyz", http.StatusMethodNotAllowed},
		{http.MethodGet, "/pipeline/v1/projects", http.StatusUnauthorized},
	} {
		response := httptest.NewRecorder()
		NewHandler(nil, nil, nil).ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, nil))
		if response.Code != tc.status {
			t.Fatalf("%s %s: status %d, want %d", tc.method, tc.path, response.Code, tc.status)
		}
	}
}

func TestServerHasResourceBounds(t *testing.T) {
	server := NewServer(nil, nil, nil)
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 || server.WriteTimeout <= 0 || server.IdleTimeout <= 0 || server.MaxHeaderBytes <= 0 {
		t.Fatal("missing HTTP resource bounds")
	}
}

func TestCanceledServerClosesListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, listener, nil, nil, nil) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after cancellation")
	}
	connection, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("listener remained open")
	}
}
