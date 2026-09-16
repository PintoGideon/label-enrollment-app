// Package client provides a nonvisual Workflow HTTP client. This first checkpoint
// implements probes only; authenticated project methods are still pending.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

const maxResponseBytes = 64 << 10

type Client struct {
	baseURL string
	http    *http.Client
}

// APIError deliberately excludes untrusted response bodies and URLs from errors.
// Callers can branch on status/code without leaking future credential data.
type APIError struct {
	StatusCode int
	Code       string
	Checks     *api.ReadinessChecks
}

func (e *APIError) Error() string {
	return fmt.Sprintf("workflow returned HTTP %d (%s)", e.StatusCode, e.Code)
}

func New(baseURL string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("workflow base URL must be an origin without credentials, path, query or fragment")
	}
	if u.Scheme != "https" {
		ip, err := netip.ParseAddr(u.Hostname())
		if u.Scheme != "http" || err != nil || !ip.IsLoopback() || ip.Zone() != "" {
			return nil, errors.New("workflow URL requires HTTPS, except for literal loopback HTTP")
		}
	}
	u.Path = ""
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{
		baseURL: u.String(),
		http: &http.Client{
			Timeout:   3 * time.Second,
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) Close() { c.http.CloseIdleConnections() }

func (c *Client) Liveness(ctx context.Context) (api.ProbeResponse, error) {
	return c.probe(ctx, "/healthz", api.StatusOK)
}

func (c *Client) Readiness(ctx context.Context) (api.ProbeResponse, error) {
	return c.probe(ctx, "/readyz", api.StatusReady)
}

func (c *Client) probe(ctx context.Context, path, expectedStatus string) (api.ProbeResponse, error) {
	var result api.ProbeResponse
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return result, errors.New("could not construct workflow request")
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, errors.New("workflow request failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(body) > maxResponseBytes {
		return result, errors.New("workflow response unreadable or exceeds size limit")
	}
	mediaType, _, _ := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK {
		code := "HTTP_ERROR"
		var problem api.ErrorResponse
		var checks *api.ReadinessChecks
		if mediaType == "application/json" && decodeJSON(body, &problem) == nil && problem.Error.Code == api.CodeNotReady && response.StatusCode == http.StatusServiceUnavailable {
			if problem.Checks != nil && !problem.Checks.Valid() {
				return result, errors.New("workflow readiness checks do not match the contract")
			}
			code = api.CodeNotReady
			checks = problem.Checks
		}
		return result, &APIError{StatusCode: response.StatusCode, Code: code, Checks: checks}
	}
	if mediaType != "application/json" || decodeJSON(body, &result) != nil || result.Status != expectedStatus || result.Service != api.ServiceName {
		return api.ProbeResponse{}, errors.New("workflow response does not match the probe contract")
	}
	return result, nil
}

func decodeJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("expected one JSON value")
	}
	return nil
}
