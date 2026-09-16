// Package database owns Workflow persistence. It never provisions test clusters,
// seeds fixtures, automatically migrates at startup, or connects without a URL.
package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"

	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/config"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/internal/database/dbsql"
	"github.com/PintoGideon/label-enrollment-app/apps/workflow/pkg/api"
)

type Store struct {
	pool     *pgxpool.Pool
	queries  *dbsql.Queries
	migrator *goose.Provider
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, nil
	}
	if err := config.ValidateDatabaseURL(databaseURL); err != nil {
		return nil, err
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		// pgx parsing errors can contain the input URI/password. Never wrap them.
		return nil, config.ErrDatabaseURL
	}
	cfg.MaxConns = 5
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 2 * time.Second
	cfg.ConnConfig.RuntimeParams["application_name"] = "label-enrollment-workflow"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "5000"
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "2000"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("could not initialize Workflow database pool")
	}
	migrator, err := newMigrator(cfg.ConnConfig)
	if err != nil {
		pool.Close()
		return nil, errors.New("could not initialize Workflow migrations")
	}
	return &Store{pool: pool, queries: dbsql.New(pool), migrator: migrator}, nil
}

func (s *Store) Close() {
	if s != nil {
		_ = s.migrator.Close()
		s.pool.Close()
	}
}

func (s *Store) Check(ctx context.Context) api.DatabaseStatus {
	if s == nil {
		return api.DatabaseNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	legacy, err := s.queries.HasLegacyMigrationLedger(ctx)
	if err != nil {
		return api.DatabaseUnavailable
	}
	if legacy {
		// No automatic adoption, reset or conversion of the retired prototype.
		return api.DatabaseSchemaMismatch
	}
	sources := s.migrator.ListSources() // Embedded files only; no database calls.
	versions, err := s.queries.ListMigrationVersions(ctx, int32(len(sources)+2))
	if err != nil {
		return schemaErrorStatus(err, api.DatabaseMigrationRequired)
	}
	if status := migrationStatus(versions, sources); status != api.DatabaseReady {
		return status
	}
	// A generated query checks required columns, not all constraint/index/type
	// drift. No probe calls Goose methods that might initialize its ledger.
	if err := s.queries.CheckProjectSchema(ctx); err != nil {
		return schemaErrorStatus(err, api.DatabaseSchemaMismatch)
	}
	if err := s.queries.CheckProcessingSchema(ctx); err != nil {
		return schemaErrorStatus(err, api.DatabaseSchemaMismatch)
	}
	return api.DatabaseReady
}

// This is a read-only application compatibility check, not a migration runner.
// Workflow only applies Up migrations, so Goose's zero row plus applied source
// versions must form an exact prefix. Future, missing and duplicate rows fail.
func migrationStatus(versions []dbsql.ListMigrationVersionsRow, sources []*goose.Source) api.DatabaseStatus {
	if len(versions) == 0 || len(versions) > len(sources)+1 {
		return api.DatabaseSchemaMismatch
	}
	for i, version := range versions {
		expected := int64(0)
		if i > 0 {
			expected = sources[i-1].Version
		}
		if version.VersionID != expected || !version.IsApplied {
			return api.DatabaseSchemaMismatch
		}
	}
	if len(versions) < len(sources)+1 {
		return api.DatabaseMigrationRequired
	}
	return api.DatabaseReady
}

func schemaErrorStatus(err error, missing api.DatabaseStatus) api.DatabaseStatus {
	var pgError *pgconn.PgError
	if errors.As(err, &pgError) {
		switch pgError.Code {
		case "42P01", "3F000": // Missing table/schema.
			return missing
		case "42703": // Incompatible columns.
			return api.DatabaseSchemaMismatch
		}
	}
	return api.DatabaseUnavailable
}
