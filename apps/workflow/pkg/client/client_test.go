package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

func TestRejectsUnsafeBaseURLs(t *testing.T) {
	for _, base := range []string{"http://example.com", "http://localhost:8080", "https://user:secret@example.com", "https://example.com/path", "https://example.com?token=secret", "https://example.com?", "https://example.com#secret", "file:///tmp/config", "://bad"} {
		t.Run(base, func(t *testing.T) {
			if _, err := New(base); err == nil {
				t.Fatal("expected invalid origin to fail")
			}
		})
	}
	for _, base := range []string{"http://127.0.0.1:8080", "http://[::1]:8080", "https://workflow.example.com/"} {
		c, err := New(base)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
	}
}

func TestValidatesResponse(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		kind string
		ok   bool
	}{
		{"valid", `{"status":"ok","service":"label-enrollment-workflow"}`, "application/json", true},
		{"wrong service", `{"status":"ok","service":"other"}`, "application/json", false},
		{"wrong status", `{"status":"ready","service":"label-enrollment-workflow"}`, "application/json", false},
		{"wrong type", `{"status":1}`, "application/json", false},
		{"malformed", `{`, "application/json", false},
		{"trailing value", `{"status":"ok","service":"label-enrollment-workflow"} {}`, "application/json", false},
		{"html", `<html>secret</html>`, "text/html", false},
		{"oversized", strings.Repeat("x", maxResponseBytes+1), "application/json", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/healthz" || r.Header.Get("Accept") != "application/json" {
					t.Error("incorrect request contract")
				}
				w.Header().Set("Content-Type", tc.kind)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			c, err := New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.Liveness(context.Background())
			if (err == nil) != tc.ok {
				t.Fatalf("got error %v; expected success %v", err, tc.ok)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("response body leaked into error")
			}
		})
	}
}

func TestErrorCodeIsBoundedAndResponseMessageNotExposed(t *testing.T) {
	for _, code := range []string{api.CodeNotReady, "secret-token"} {
		t.Run(code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				fmt.Fprintf(w, `{"error":{"code":%q,"message":"secret-token"}}`, code)
			}))
			defer server.Close()
			c, err := New(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.Readiness(context.Background())
			var problem *APIError
			if !errors.As(err, &problem) || problem.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("expected structured API error, got %v", err)
			}
			want := "HTTP_ERROR"
			if code == api.CodeNotReady {
				want = api.CodeNotReady
			}
			if problem.Code != want || strings.Contains(err.Error(), "secret-token") {
				t.Fatal("untrusted response details escaped error handling")
			}
		})
	}
}

func TestDoesNotFollowRedirects(t *testing.T) {
	var followed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		followed.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()
	c, err := New(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, err = c.Liveness(context.Background())
	var problem *APIError
	if !errors.As(err, &problem) || problem.StatusCode != http.StatusFound || followed.Load() {
		t.Fatal("redirect was followed or reported as success")
	}
}

func TestHonorsContextCancellationAndTimeout(t *testing.T) {
	c, err := New("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.http.Timeout <= 0 {
		t.Fatal("missing request timeout")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.Liveness(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
