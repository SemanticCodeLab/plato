package store

import "database/sql"

// Page is a page index row. Content lives in the Markdown file at rel_path; only
// metadata and content_hash are stored here.
type Page struct {
	ID          int64          `json:"id"`
	WikiID      int64          `json:"wiki_id"`
	Slug        string         `json:"slug"`
	Title       string         `json:"title"`
	RelPath     string         `json:"rel_path"`
	ContentHash string         `json:"content_hash"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
	DeletedAt   sql.NullString `json:"-"`
}

const pageCols = `id, wiki_id, slug, title, rel_path, content_hash, created_at, updated_at, deleted_at`

func scanPage(s interface{ Scan(...any) error }) (*Page, error) {
	var p Page
	if err := s.Scan(&p.ID, &p.WikiID, &p.Slug, &p.Title, &p.RelPath, &p.ContentHash,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// CreatePage inserts a new page row.
func (db *DB) CreatePage(wikiID int64, slug, title, relPath, contentHash string) (*Page, error) {
	ts := now()
	res, err := db.Exec(
		`INSERT INTO pages (wiki_id, slug, title, rel_path, content_hash, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		wikiID, slug, title, relPath, contentHash, ts, ts)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.PageByID(id)
}

// UpdatePage updates a page's mutable metadata (title, slug, content_hash).
func (db *DB) UpdatePage(id int64, slug, title, contentHash string) error {
	_, err := db.Exec(
		`UPDATE pages SET slug = ?, title = ?, content_hash = ?, updated_at = ?, deleted_at = NULL
		 WHERE id = ?`,
		slug, title, contentHash, now(), id)
	return err
}

// SoftDeletePage marks a page deleted without removing the row.
func (db *DB) SoftDeletePage(id int64) error {
	_, err := db.Exec(`UPDATE pages SET deleted_at = ? WHERE id = ?`, now(), id)
	return err
}

// PageByID returns a page by id (including soft-deleted), or (nil, nil) if absent.
func (db *DB) PageByID(id int64) (*Page, error) {
	p, err := scanPage(db.QueryRow(`SELECT `+pageCols+` FROM pages WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// PageBySlug returns a live (not deleted) page by wiki + slug, or (nil, nil).
func (db *DB) PageBySlug(wikiID int64, slug string) (*Page, error) {
	p, err := scanPage(db.QueryRow(
		`SELECT `+pageCols+` FROM pages WHERE wiki_id = ? AND slug = ? AND deleted_at IS NULL`,
		wikiID, slug))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// PageByRelPath returns a page (including soft-deleted) by wiki + rel_path. Page
// identity is wiki_id + rel_path, so sync upserts key on this.
func (db *DB) PageByRelPath(wikiID int64, relPath string) (*Page, error) {
	p, err := scanPage(db.QueryRow(
		`SELECT `+pageCols+` FROM pages WHERE wiki_id = ? AND rel_path = ?`, wikiID, relPath))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// PageByTitle returns live pages in a wiki whose title matches exactly (used for
// wikilink resolution; may return several for ambiguity detection).
func (db *DB) PagesByTitle(wikiID int64, title string, caseInsensitive bool) ([]Page, error) {
	q := `SELECT ` + pageCols + ` FROM pages WHERE wiki_id = ? AND deleted_at IS NULL AND title = ?`
	if caseInsensitive {
		q = `SELECT ` + pageCols + ` FROM pages WHERE wiki_id = ? AND deleted_at IS NULL AND title = ? COLLATE NOCASE`
	}
	rows, err := db.Query(q, wikiID, title)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ListPages returns all live pages in a wiki ordered by slug.
func (db *DB) ListPages(wikiID int64) ([]Page, error) {
	rows, err := db.Query(
		`SELECT `+pageCols+` FROM pages WHERE wiki_id = ? AND deleted_at IS NULL ORDER BY slug`,
		wikiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Page
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// SlugExists reports whether a live page with the slug exists in the wiki,
// excluding the page with excludeID (pass 0 to exclude none). Used for slug dedup.
func (db *DB) SlugExists(wikiID int64, slug string, excludeID int64) (bool, error) {
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pages WHERE wiki_id = ? AND slug = ? AND id != ? AND deleted_at IS NULL`,
		wikiID, slug, excludeID).Scan(&n)
	return n > 0, err
}
