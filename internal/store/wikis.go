package store

import "database/sql"

// Source types for a project (wiki).
const (
	SourceEmpty = "empty"
	SourceLocal = "local"
	SourceGit   = "git"
)

// Wiki is a project row. root_path is the absolute directory holding its Markdown
// files; the source_* fields record where the content was imported from.
type Wiki struct {
	ID            int64          `json:"id"`
	Slug          string         `json:"slug"`
	Title         string         `json:"title"`
	RootPath      string         `json:"-"`
	SourceType    string         `json:"source_type"`
	SourceURL     sql.NullString `json:"-"`
	SourceBranch  sql.NullString `json:"-"`
	SourceSubdir  sql.NullString `json:"-"`
	LastIndexedAt sql.NullString `json:"-"`
	CreatedAt     string         `json:"created_at"`
}

// Marshaling helpers so null columns serialize as omitted/empty strings.
func (w Wiki) MarshalSource() map[string]any {
	return map[string]any{
		"source_url":      w.SourceURL.String,
		"source_branch":   w.SourceBranch.String,
		"source_subdir":   w.SourceSubdir.String,
		"last_indexed_at": w.LastIndexedAt.String,
	}
}

const wikiCols = `id, slug, title, root_path, source_type, source_url, source_branch, source_subdir, last_indexed_at, created_at`

func scanWiki(s interface{ Scan(...any) error }) (*Wiki, error) {
	var w Wiki
	if err := s.Scan(&w.ID, &w.Slug, &w.Title, &w.RootPath, &w.SourceType,
		&w.SourceURL, &w.SourceBranch, &w.SourceSubdir, &w.LastIndexedAt, &w.CreatedAt); err != nil {
		return nil, err
	}
	return &w, nil
}

// CreateWiki inserts a project with source metadata and returns the new row id.
func (db *DB) CreateWiki(slug, title, rootPath, sourceType, sourceURL, branch, subdir string) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO wikis (slug, title, root_path, source_type, source_url, source_branch, source_subdir, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		slug, title, rootPath, sourceType,
		nullIf(sourceURL), nullIf(branch), nullIf(subdir), now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetLastIndexed records when a project's pages/links were last (re)built.
func (db *DB) SetLastIndexed(wikiID int64) error {
	_, err := db.Exec(`UPDATE wikis SET last_indexed_at = ? WHERE id = ?`, now(), wikiID)
	return err
}

// WikiBySlug returns the project with the given slug, or (nil, nil) if absent.
func (db *DB) WikiBySlug(slug string) (*Wiki, error) {
	w, err := scanWiki(db.QueryRow(`SELECT `+wikiCols+` FROM wikis WHERE slug = ?`, slug))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return w, err
}

// ListWikis returns all projects ordered by slug.
func (db *DB) ListWikis() ([]Wiki, error) {
	rows, err := db.Query(`SELECT ` + wikiCols + ` FROM wikis ORDER BY slug`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Wiki
	for rows.Next() {
		w, err := scanWiki(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func nullIf(s string) any {
	if s == "" {
		return nil
	}
	return s
}
