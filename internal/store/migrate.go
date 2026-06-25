package store

import (
	_ "embed"
	"strings"
)

//go:embed schema.sql
var schemaSQL string

const schemaVersion = 3

// migrate applies the base schema then incremental column additions. Both the
// base schema and the ALTERs are written to be idempotent so this is safe to run
// on every startup.
func (db *DB) migrate() error {
	if _, err := db.Exec(schemaSQL); err != nil {
		return err
	}

	// v2: project source metadata on wikis. SQLite lacks "ADD COLUMN IF NOT
	// EXISTS", so add each column and ignore the duplicate-column error.
	v2 := []string{
		`ALTER TABLE wikis ADD COLUMN source_type TEXT NOT NULL DEFAULT 'empty'`,
		`ALTER TABLE wikis ADD COLUMN source_url TEXT`,
		`ALTER TABLE wikis ADD COLUMN source_branch TEXT`,
		`ALTER TABLE wikis ADD COLUMN source_subdir TEXT`,
		`ALTER TABLE wikis ADD COLUMN last_indexed_at TEXT`,
	}
	for _, stmt := range v2 {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumn(err) {
			return err
		}
	}

	// v3: link origin (auto-discovered vs manually added via the API).
	v3 := []string{
		`ALTER TABLE links ADD COLUMN origin TEXT NOT NULL DEFAULT 'auto'`,
	}
	for _, stmt := range v3 {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumn(err) {
			return err
		}
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_version`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (?)`, schemaVersion); err != nil {
			return err
		}
	} else {
		if _, err := db.Exec(`UPDATE schema_version SET version = ?`, schemaVersion); err != nil {
			return err
		}
	}
	return nil
}

// isDuplicateColumn reports whether err is SQLite's "duplicate column name".
func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}
