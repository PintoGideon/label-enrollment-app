package config

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// Auth is resource-server configuration, not a login client or credential store.
// All zero values disable verification; protected routes still fail closed.
type Auth struct {
	Issuer   string
	Audience string
	JWKSURL  string
	CABundle string // Optional additional PEM roots; never disables TLS verification.
}

var ErrAuthConfig = errors.New("Workflow authentication requires an HTTPS issuer, audience and HTTPS JWKS URL; CA trust is optional")

func (a Auth) Enabled() bool { return a != (Auth{}) }

func (a Auth) Validate() error {
	if !a.Enabled() {
		return nil
	}
	if !httpsResource(a.Issuer, 512) || !httpsResource(a.JWKSURL, 2048) || len(a.Audience) == 0 || len(a.Audience) > 512 || strings.ContainsFunc(a.Audience, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return ErrAuthConfig
	}
	return nil
}

func httpsResource(raw string, maxLength int) bool {
	if len(raw) == 0 || len(raw) > maxLength {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(u.Host, "%") {
		return false
	}
	if strings.HasSuffix(u.Host, ":") {
		return false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return true
}
