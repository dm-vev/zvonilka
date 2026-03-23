package storage

import "database/sql"

// RelationalProvider exposes a transactional SQL handle for relational workloads.
type RelationalProvider interface {
	Provider
	DB() *sql.DB
}
