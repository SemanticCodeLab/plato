package store

import "database/sql"

// Audit records an audit event. wikiID/pageID may be 0 to store NULL.
func (db *DB) Audit(actor, action string, wikiID, pageID int64, metadataJSON string) error {
	var w, p sql.NullInt64
	if wikiID != 0 {
		w = sql.NullInt64{Int64: wikiID, Valid: true}
	}
	if pageID != 0 {
		p = sql.NullInt64{Int64: pageID, Valid: true}
	}
	var meta sql.NullString
	if metadataJSON != "" {
		meta = sql.NullString{String: metadataJSON, Valid: true}
	}
	_, err := db.Exec(
		`INSERT INTO audit_events (actor, action, wiki_id, page_id, metadata_json, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		actor, action, w, p, meta, now())
	return err
}
