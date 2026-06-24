// Package store is the SQLite index for Plato. Markdown files on disk remain the
// source of truth; this package indexes page metadata, the cross-link graph, and
// API tokens.
package store

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps *sql.DB. SQLite is the only backend in the MVP, so queries use native
// "?" placeholders directly.
type DB struct {
	*sql.DB
}

// Open opens (creating if needed) the SQLite database at path and applies the
// schema migration.
func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	// SQLite handles concurrency best with a single writer connection.
	sqlDB.SetMaxOpenConns(1)
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	db := &DB{sqlDB}
	if err := db.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// now returns an RFC3339 UTC timestamp string, the format used for all *_at columns.
func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
