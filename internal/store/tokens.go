package store

import "database/sql"

// Token is a stored API token row. The raw token value is never persisted; only
// its hash is.
type Token struct {
	ID         int64
	Name       string
	Scopes     string // comma-separated, e.g. "read,write"
	CreatedAt  string
	LastUsedAt sql.NullString
	RevokedAt  sql.NullString
}

// CountTokens returns the number of token rows (used to decide bootstrap).
func (db *DB) CountTokens() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM api_tokens`).Scan(&n)
	return n, err
}

// CreateToken inserts a token by its hash and returns the new row id.
func (db *DB) CreateToken(name, tokenHash, scopes string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO api_tokens (name, token_hash, scopes, created_at) VALUES (?, ?, ?, ?)`,
		name, tokenHash, scopes, now(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// TokenByHash looks up a non-revoked token by its hash, recording last use.
// Returns (nil, nil) when no active token matches.
func (db *DB) TokenByHash(tokenHash string) (*Token, error) {
	var t Token
	err := db.QueryRow(
		`SELECT id, name, scopes, created_at, last_used_at, revoked_at
		   FROM api_tokens WHERE token_hash = ? AND revoked_at IS NULL`,
		tokenHash,
	).Scan(&t.ID, &t.Name, &t.Scopes, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, _ = db.Exec(`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, now(), t.ID)
	return &t, nil
}

// ListTokens returns all tokens (without hashes).
func (db *DB) ListTokens() ([]Token, error) {
	rows, err := db.Query(
		`SELECT id, name, scopes, created_at, last_used_at, revoked_at
		   FROM api_tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Token
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.Name, &t.Scopes, &t.CreatedAt, &t.LastUsedAt, &t.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RevokeToken marks a token revoked. Returns false if no such active token.
func (db *DB) RevokeToken(id int64) (bool, error) {
	res, err := db.Exec(
		`UPDATE api_tokens SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		now(), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
