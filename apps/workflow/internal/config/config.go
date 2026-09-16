// Package config validates configuration before opening a listener.
package config

import (
	"errors"
	"net"
	"net/netip"
	"strconv"
)

type Config struct {
	Address     string
	DatabaseURL string `json:"-"`
}

// Load deliberately permits only literal loopback addresses for this local-only
// foundation. Public listeners require a later, authenticated deployment slice.
func Load(getenv func(string) string) (Config, error) {
	address := getenv("WORKFLOW_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	host, port, err := net.SplitHostPort(address)
	ip, ipErr := netip.ParseAddr(host)
	number, portErr := strconv.Atoi(port)
	if err != nil || ipErr != nil || !ip.IsLoopback() || ip.Zone() != "" || portErr != nil || number < 0 || number > 65535 {
		return Config{}, errors.New("WORKFLOW_ADDR must be a literal loopback IP and port between 0 and 65535")
	}
	databaseURL := getenv("WORKFLOW_DATABASE_URL")
	if err := ValidateDatabaseURL(databaseURL); err != nil {
		return Config{}, err
	}
	return Config{Address: net.JoinHostPort(host, strconv.Itoa(number)), DatabaseURL: databaseURL}, nil
}
