package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	for _, address := range []string{"", "127.0.0.1:8080", "127.0.0.1:0", "[::1]:8080"} {
		t.Run(address, func(t *testing.T) {
			cfg, err := Load(func(key string) string {
				if key == "WORKFLOW_ADDR" {
					return address
				}
				return ""
			})
			if err != nil {
				t.Fatal(err)
			}
			want := address
			if want == "" {
				want = "127.0.0.1:8080"
			}
			if cfg.Address != want {
				t.Fatalf("address = %q; want %q", cfg.Address, want)
			}
		})
	}
}

func TestRejectsPublicOrMalformedListenersWithoutEchoingInput(t *testing.T) {
	for _, address := range []string{"0.0.0.0:8080", "[::]:8080", "localhost:8080", "192.168.1.2:8080", "127.0.0.1:-1", "127.0.0.1:65536", "127.0.0.1:http", "private-secret"} {
		t.Run(address, func(t *testing.T) {
			_, err := Load(func(string) string { return address })
			if err == nil {
				t.Fatal("expected invalid configuration to fail")
			}
			if strings.Contains(err.Error(), address) {
				t.Fatal("configuration error echoed its input")
			}
		})
	}
}
