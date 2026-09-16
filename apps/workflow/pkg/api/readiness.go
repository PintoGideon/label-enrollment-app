package api

// DatabaseStatus reports dependency/schema readiness, not project authorization.
type DatabaseStatus string

const (
	DatabaseNotConfigured     DatabaseStatus = "not_configured"
	DatabaseUnavailable       DatabaseStatus = "unavailable"
	DatabaseMigrationRequired DatabaseStatus = "migration_required"
	DatabaseSchemaMismatch    DatabaseStatus = "schema_mismatch"
	DatabaseReady             DatabaseStatus = "ready"
	ProjectAPINotImplemented                 = "not_implemented"
)

type ReadinessChecks struct {
	Database   DatabaseStatus `json:"database"`
	ProjectAPI string         `json:"projectApi"`
}

func (s DatabaseStatus) Valid() bool {
	switch s {
	case DatabaseNotConfigured, DatabaseUnavailable, DatabaseMigrationRequired, DatabaseSchemaMismatch, DatabaseReady:
		return true
	default:
		return false
	}
}

func (c ReadinessChecks) Valid() bool {
	return c.Database.Valid() && (c.ProjectAPI == ProjectAPINotImplemented || c.ProjectAPI == StatusReady)
}
