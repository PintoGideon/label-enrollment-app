package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pressly/goose/v3"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/database/dbsql"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

func TestMigrationCompatibility(t *testing.T) {
	sources := []*goose.Source{{Version: 1}, {Version: 3}}
	zero := dbsql.ListMigrationVersionsRow{VersionID: 0, IsApplied: true}
	one := dbsql.ListMigrationVersionsRow{VersionID: 1, IsApplied: true}
	three := dbsql.ListMigrationVersionsRow{VersionID: 3, IsApplied: true}
	for _, tc := range []struct {
		name     string
		versions []dbsql.ListMigrationVersionsRow
		want     api.DatabaseStatus
	}{
		{"empty ledger is corrupt", nil, api.DatabaseSchemaMismatch},
		{"initialized", []dbsql.ListMigrationVersionsRow{zero}, api.DatabaseMigrationRequired},
		{"pending", []dbsql.ListMigrationVersionsRow{zero, one}, api.DatabaseMigrationRequired},
		{"current", []dbsql.ListMigrationVersionsRow{zero, one, three}, api.DatabaseReady},
		{"missing zero", []dbsql.ListMigrationVersionsRow{one, three}, api.DatabaseSchemaMismatch},
		{"gap", []dbsql.ListMigrationVersionsRow{zero, three}, api.DatabaseSchemaMismatch},
		{"unknown", []dbsql.ListMigrationVersionsRow{zero, {VersionID: 2, IsApplied: true}}, api.DatabaseSchemaMismatch},
		{"future", []dbsql.ListMigrationVersionsRow{zero, one, three, {VersionID: 4, IsApplied: true}}, api.DatabaseSchemaMismatch},
		{"duplicate", []dbsql.ListMigrationVersionsRow{zero, one, one}, api.DatabaseSchemaMismatch},
		{"unapplied", []dbsql.ListMigrationVersionsRow{zero, {VersionID: 1}}, api.DatabaseSchemaMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := migrationStatus(tc.versions, sources); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestEmbeddedGooseSourcesRequireNoConnection(t *testing.T) {
	// This address has no fixture server: construction must not connect/migrate.
	store, err := Open(context.Background(), "postgres://test:test@127.0.0.1:1/test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sources := store.migrator.ListSources()
	if len(sources) != 1 || sources[0].Version != 1 || sources[0].Path != "001_projects.sql" {
		t.Fatal("Goose did not discover the expected embedded SQL source")
	}
	for _, source := range sources {
		sql, err := migrationFiles.ReadFile("migrations/" + source.Path)
		if err != nil || !strings.Contains(string(sql), "-- +goose Up\n") || strings.Contains(string(sql), "\r") || strings.Contains(string(sql), "-- +goose NO TRANSACTION") {
			t.Fatal("migration must be canonical LF, Goose SQL and transactional")
		}
	}
}

type unlockDeadlineProbe struct {
	called bool
}

func (*unlockDeadlineProbe) SessionLock(context.Context, *sql.Conn) error { return nil }

func (p *unlockDeadlineProbe) SessionUnlock(ctx context.Context, _ *sql.Conn) error {
	p.called = true
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) <= 0 || time.Until(deadline) > 3*time.Second {
		return errors.New("unlock query must have a bounded deadline")
	}
	return nil
}

func TestMigrationUnlockIsBounded(t *testing.T) {
	probe := &unlockDeadlineProbe{}
	if err := (boundedSessionLocker{probe}).SessionUnlock(context.Background(), nil); err != nil || !probe.called {
		t.Fatalf("cleanup deadline not enforced: %v", err)
	}
}

func TestSchemaErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want api.DatabaseStatus
	}{
		{&pgconn.PgError{Code: "42P01"}, api.DatabaseMigrationRequired},
		{&pgconn.PgError{Code: "3F000"}, api.DatabaseMigrationRequired},
		{&pgconn.PgError{Code: "42703"}, api.DatabaseSchemaMismatch},
		{&pgconn.PgError{Code: "42501"}, api.DatabaseUnavailable},
		{context.DeadlineExceeded, api.DatabaseUnavailable},
		{errors.New("synthetic-private-error"), api.DatabaseUnavailable},
	} {
		if got := schemaErrorStatus(tc.err, api.DatabaseMigrationRequired); got != tc.want {
			t.Fatalf("got %s, want %s", got, tc.want)
		}
	}
}

func TestUnconfiguredDatabaseCannotConnectOrMigrate(t *testing.T) {
	t.Setenv("PGHOST", "must-not-connect.example")
	store, err := Open(context.Background(), "")
	if err != nil || store != nil || store.Check(context.Background()) != api.DatabaseNotConfigured {
		t.Fatal("missing URL did not disable the adapter")
	}
	if _, err := store.Migrate(context.Background()); err == nil {
		t.Fatal("migrations require explicit database configuration")
	}
	store.Close()
}

func TestInvalidDatabaseDoesNotLeakInput(t *testing.T) {
	_, err := Open(context.Background(), "synthetic-private-marker")
	if err == nil || strings.Contains(err.Error(), "synthetic-private-marker") {
		t.Fatal("invalid input accepted or leaked")
	}
}
