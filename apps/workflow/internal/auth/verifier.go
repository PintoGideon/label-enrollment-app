// Package auth verifies access tokens for one explicitly configured AuthD issuer
// and Workflow audience. It neither issues tokens nor stores user credentials.
package auth

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/config"
)

const (
	MaxTokenBytes = 8 << 10
	maxJWKSBytes  = 64 << 10
	cacheMaxAge   = 5 * time.Minute
	refreshDelay  = 30 * time.Second
)

var (
	ErrInvalid     = errors.New("invalid Workflow access token")
	ErrUnavailable = errors.New("Workflow authentication is unavailable")
)

type Principal struct {
	Issuer  string
	Subject string
}

func (p Principal) Valid() bool {
	return p.Issuer != "" && len(p.Issuer) <= 512 && utf8.ValidString(p.Subject) &&
		utf8.RuneCountInString(p.Subject) <= 255 && strings.TrimSpace(p.Subject) != "" &&
		!strings.ContainsFunc(p.Subject, unicode.IsControl)
}

type Verifier struct {
	cfg       config.Auth
	http      *http.Client
	gate      chan struct{} // Serializes refresh/cache access; acquisition is cancelable.
	keys      keyfunc.Keyfunc
	fetchedAt time.Time
	nextFetch time.Time
	failed    bool
}

// New validates local configuration/trust only. Keys are fetched lazily; issuer
// outages never prevent liveness or cause startup to write to the database.
func New(cfg config.Auth) (*Verifier, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled() {
		return nil, nil
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 2 * time.Second
	transport.MaxResponseHeaderBytes = 8 << 10
	transport.MaxConnsPerHost = 1
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CABundle != "" {
		roots, err := extraRoots(cfg.CABundle)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	return &Verifier{
		cfg: cfg, gate: make(chan struct{}, 1),
		http: &http.Client{
			Transport: transport, Timeout: 2 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (v *Verifier) Close() {
	if v != nil {
		v.http.CloseIdleConnections()
	}
}

func (v *Verifier) Verify(ctx context.Context, raw string) (Principal, error) {
	if err := ctx.Err(); err != nil {
		return Principal{}, err
	}
	if raw == "" || len(raw) > MaxTokenBytes || strings.ContainsAny(raw, " \t\r\n") {
		return Principal{}, ErrInvalid
	}
	if v == nil {
		return Principal{}, ErrUnavailable
	}
	claims := &jwt.RegisteredClaims{}
	options := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"EdDSA", "RS256", "ES256"}),
		jwt.WithIssuer(v.cfg.Issuer), jwt.WithAudience(v.cfg.Audience),
		jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithLeeway(30 * time.Second),
		jwt.WithStrictDecoding(),
	}
	validator := jwt.NewValidator(options...)
	_, err := jwt.NewParser(options...).ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) {
		// Reject unsupported header extensions. Neither tokens nor JWKS can
		// introduce another key URL. Unverified claims only reject early here;
		// they never authorize a caller before the signature is checked.
		for _, name := range []string{"crit", "jku", "jwk", "x5u", "x5c"} {
			if _, exists := token.Header[name]; exists {
				return nil, ErrInvalid
			}
		}
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" || len(kid) > 128 || !(Principal{Issuer: claims.Issuer, Subject: claims.Subject}).Valid() || validator.Validate(claims) != nil {
			return nil, ErrInvalid
		}
		return v.key(ctx, kid, token)
	})
	if ctx.Err() != nil {
		return Principal{}, ctx.Err()
	}
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			return Principal{}, ErrUnavailable
		}
		return Principal{}, ErrInvalid
	}
	return Principal{Issuer: claims.Issuer, Subject: claims.Subject}, nil
}

func (v *Verifier) key(ctx context.Context, kid string, token *jwt.Token) (any, error) {
	select {
	case v.gate <- struct{}{}:
		defer func() { <-v.gate }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := time.Now()
	fresh := v.keys != nil && now.Sub(v.fetchedAt) < cacheMaxAge
	if fresh {
		if _, err := v.keys.Storage().KeyRead(ctx, kid); err == nil {
			return v.keys.KeyfuncCtx(ctx)(token)
		}
	}
	if now.Before(v.nextFetch) {
		if !fresh || v.failed {
			return nil, ErrUnavailable
		}
		return nil, ErrInvalid
	}
	// Global cooldown (including failed requests) bounds unknown-kid traffic.
	// No unbounded negative-key map, background worker or stale-key fallback.
	v.nextFetch = now.Add(refreshDelay)
	keys, err := v.fetch(ctx)
	if err != nil {
		v.failed = true
		return nil, ErrUnavailable
	}
	v.keys, v.fetchedAt, v.failed = keys, time.Now(), false
	return v.keys.KeyfuncCtx(ctx)(token)
}

func (v *Verifier) fetch(ctx context.Context) (keyfunc.Keyfunc, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return nil, ErrUnavailable
	}
	req.Header.Set("Accept", "application/json")
	resp, err := v.http.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBytes+1))
	if err != nil || len(body) > maxJWKSBytes {
		return nil, ErrUnavailable
	}
	var set jwkset.JWKSMarshal
	if json.Unmarshal(body, &set) != nil || len(set.Keys) == 0 || len(set.Keys) > 32 {
		return nil, ErrUnavailable
	}
	seen := make(map[string]bool, len(set.Keys))
	for _, k := range set.Keys {
		if k.KID == "" || len(k.KID) > 128 || seen[k.KID] || k.X5U != "" || k.D != "" || k.K != "" || (k.USE != "" && k.USE != "sig") {
			return nil, ErrUnavailable
		}
		if k.ALG != "" && k.ALG != "EdDSA" && k.ALG != "RS256" && k.ALG != "ES256" {
			return nil, ErrUnavailable
		}
		for _, operation := range k.KEYOPS {
			if operation != "verify" {
				return nil, ErrUnavailable
			}
		}
		seen[k.KID] = true
	}
	keys, err := keyfunc.NewJWKSetJSON(body)
	if err != nil {
		return nil, ErrUnavailable
	}
	parsed, err := keys.Storage().KeyReadAll(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	for _, k := range parsed {
		if pub, ok := k.Key().(*rsa.PublicKey); ok && (pub.N.BitLen() < 2048 || pub.N.BitLen() > 8192) {
			return nil, ErrUnavailable
		}
	}
	return keys, nil
}

func extraRoots(path string) (*x509.CertPool, error) {
	const maxCABytes = 1 << 20
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxCABytes {
		return nil, errors.New("could not load Workflow authentication CA trust")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("could not load Workflow authentication CA trust")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCABytes+1))
	if err != nil || len(data) > maxCABytes {
		return nil, errors.New("could not load Workflow authentication CA trust")
	}
	roots, err := x509.SystemCertPool()
	if err != nil || !roots.AppendCertsFromPEM(data) {
		return nil, errors.New("could not load Workflow authentication CA trust")
	}
	return roots, nil
}
