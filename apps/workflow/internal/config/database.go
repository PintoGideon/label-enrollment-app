package config

import (
	"errors"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var ErrDatabaseURL = errors.New("WORKFLOW_DATABASE_URL requires a complete loopback PostgreSQL URL with user, password, port, database and explicit sslmode")

// ValidateDatabaseURL permits only explicit local connections for this increment.
// Cloud database configuration/TLS policy is a later deployment decision. An empty
// URL disables the adapter; it must never fall back to PGHOST or a user's database.
func ValidateDatabaseURL(raw string) error {
	if raw == "" {
		return nil
	}
	if len(raw) > 8192 {
		return ErrDatabaseURL
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil || u.User.Username() == "" || u.Fragment != "" {
		return ErrDatabaseURL
	}
	password, present := u.User.Password()
	ip, ipErr := netip.ParseAddr(u.Hostname())
	port, portErr := strconv.Atoi(u.Port())
	database := strings.TrimPrefix(u.Path, "/")
	query, queryErr := url.ParseQuery(u.RawQuery)
	if !present || password == "" || ipErr != nil || !ip.IsLoopback() || ip.Zone() != "" || portErr != nil || port < 1 || port > 65535 || database == "" || strings.Contains(database, "/") || queryErr != nil || len(query) != 1 || len(query["sslmode"]) != 1 {
		return ErrDatabaseURL
	}
	switch query.Get("sslmode") {
	case "disable", "require", "verify-full":
		return nil
	default:
		return ErrDatabaseURL
	}
}
