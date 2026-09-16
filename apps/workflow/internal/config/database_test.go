package config

import (
	"strings"
	"testing"
)

func TestExplicitDatabaseConfiguration(t *testing.T) {
	valid := []string{
		"",
		"postgres://user:synthetic-password@127.0.0.1:5432/workflow?sslmode=disable",
		"postgresql://user:synthetic-password@[::1]:5432/workflow?sslmode=require",
	}
	for _, value := range valid {
		cfg, err := Load(func(key string) string {
			if key == "WORKFLOW_DATABASE_URL" {
				return value
			}
			return ""
		})
		if err != nil || cfg.DatabaseURL != value {
			t.Fatal("valid explicit database configuration rejected")
		}
	}
}

func TestDatabaseURLRejectsImplicitRemoteOrOverriddenConnections(t *testing.T) {
	for _, value := range []string{
		"synthetic-secret",
		"postgres://user:synthetic-secret@remote.example:5432/db?sslmode=require",
		"postgres://user:synthetic-secret@localhost:5432/db?sslmode=disable",
		"postgres://user@127.0.0.1:5432/db?sslmode=disable",
		"postgres://user:synthetic-secret@127.0.0.1/db?sslmode=disable",
		"postgres://user:synthetic-secret@127.0.0.1:5432/?sslmode=disable",
		"postgres://user:synthetic-secret@127.0.0.1:5432/db",
		"postgres://user:synthetic-secret@127.0.0.1:5432/db?sslmode=prefer",
		"postgres://user:synthetic-secret@127.0.0.1:5432/db?sslmode=disable&host=remote.example",
		"postgres://user:synthetic-secret@127.0.0.1:5432/db?sslmode=disable&sslmode=require",
	} {
		if err := ValidateDatabaseURL(value); err == nil || strings.Contains(err.Error(), "synthetic-secret") {
			t.Fatal("invalid configuration accepted or input leaked into error")
		}
	}
}

func TestDoesNotUseGenericDatabaseEnvironment(t *testing.T) {
	cfg, err := Load(func(key string) string {
		if key == "DATABASE_URL" || key == "PGHOST" || key == "PGDATABASE" {
			t.Fatal("must not look up another application's database")
		}
		return ""
	})
	if err != nil || cfg.DatabaseURL != "" {
		t.Fatal("missing explicit URL should disable the adapter")
	}
}
