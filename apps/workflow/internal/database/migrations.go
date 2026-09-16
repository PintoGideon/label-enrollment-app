package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const migrationTable = "public.workflow_goose_db_version"

// Goose detaches cancellation during unlock. Its retry count does not bound an
// individual query, so retain a deadline for cleanup as well as migration work.
type boundedSessionLocker struct{ lock.SessionLocker }

func (l boundedSessionLocker) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return l.SessionLocker.SessionUnlock(ctx, conn)
}

func newMigrator(cfg *pgx.ConnConfig) (*goose.Provider, error) {
	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	locker, err := lock.NewPostgresSessionLocker(
		lock.WithLockID(1279345234),
		lock.WithLockTimeout(1, 20),
		lock.WithUnlockTimeout(1, 2),
	)
	if err != nil {
		return nil, err
	}
	// Goose uses database/sql through pgx. Keep migration sessions separate from
	// the application pool; closing this handle also releases any session lock
	// left after a failed unlock. OpenDB does not connect or initialize tables.
	db := stdlib.OpenDB(*cfg.Copy())
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(30 * time.Minute)
	provider, err := goose.NewProvider(goose.DialectPostgres, db, files,
		goose.WithTableName(migrationTable),
		goose.WithSessionLocker(boundedSessionLocker{locker}),
		goose.WithDisableGlobalRegistry(true),
		// Goose/parser/driver errors may contain SQL or credentials. Only our
		// sanitized command-level outcome is logged, never raw dependency output.
		goose.WithSlog(slog.New(slog.NewTextHandler(io.Discard, nil))),
	)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return provider, nil
}

// Migrate is an explicit operational action, never called by Open or probes.
// Goose owns SQL parsing, locking, transactions and its version table. Each
// migration is its own transaction; this is not an all-files atomic batch.
func (s *Store) Migrate(ctx context.Context) (int, error) {
	if s == nil {
		return 0, errors.New("WORKFLOW_DATABASE_URL is required for migrations")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	switch s.Check(ctx) {
	case api.DatabaseReady, api.DatabaseMigrationRequired:
		// A fresh/pending database or an idempotent replay is permitted.
	case api.DatabaseSchemaMismatch:
		return 0, errors.New("Workflow schema or migration history is incompatible; no migrations attempted")
	default:
		return 0, errors.New("Workflow database is unavailable; no migrations attempted")
	}
	results, err := s.migrator.Up(ctx)
	if err != nil {
		// Earlier files may already be committed. A lost COMMIT acknowledgement
		// is uncertain, not proof of rollback. Never blindly claim a clean reset.
		return 0, errors.New("Workflow migrations failed or completion was not confirmed; inspect database status before retrying")
	}
	if s.Check(ctx) != api.DatabaseReady {
		return 0, errors.New("Workflow schema check failed after migrations; applied changes may remain")
	}
	return len(results), nil
}
