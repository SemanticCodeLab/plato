package store

import "database/sql"

// Link statuses and kinds.
const (
	StatusResolved  = "resolved"
	StatusMissing   = "missing"
	StatusAmbiguous = "ambiguous"

	KindWiki     = "wiki"
	KindRelative = "relative"
)

// Link is one outgoing cross-link from a page.
type Link struct {
	ID         int64
	WikiID     int64
	FromPageID int64
	ToPageID   sql.NullInt64
	Target     string
	Raw        string
	Label      sql.NullString
	Kind       string
	Status     string
	CreatedAt  string
}

// LinkView is an enriched outgoing link for the API, with the resolved target slug.
type LinkView struct {
	Raw    string `json:"raw"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	ToSlug string `json:"to_slug,omitempty"`
}

// Backlink is an inbound link for the API.
type Backlink struct {
	FromSlug string `json:"from_slug"`
	Raw      string `json:"raw"`
	Status   string `json:"status"`
}

// DeleteLinksFrom removes all outgoing links recorded for a page (used before
// reindexing that page).
func (db *DB) DeleteLinksFrom(fromPageID int64) error {
	_, err := db.Exec(`DELETE FROM links WHERE from_page_id = ?`, fromPageID)
	return err
}

// InsertLink records one outgoing link.
func (db *DB) InsertLink(l Link) error {
	_, err := db.Exec(
		`INSERT INTO links (wiki_id, from_page_id, to_page_id, target, raw, label, kind, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		l.WikiID, l.FromPageID, l.ToPageID, l.Target, l.Raw, l.Label, l.Kind, l.Status, now())
	return err
}

// OutgoingLinks returns enriched outgoing links for a page.
func (db *DB) OutgoingLinks(fromPageID int64) ([]LinkView, error) {
	rows, err := db.Query(
		`SELECT l.raw, l.target, l.label, l.kind, l.status, p.slug
		   FROM links l LEFT JOIN pages p ON p.id = l.to_page_id
		  WHERE l.from_page_id = ? ORDER BY l.id`, fromPageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LinkView{}
	for rows.Next() {
		var lv LinkView
		var label, slug sql.NullString
		if err := rows.Scan(&lv.Raw, &lv.Target, &label, &lv.Kind, &lv.Status, &slug); err != nil {
			return nil, err
		}
		lv.Label = label.String
		lv.ToSlug = slug.String
		out = append(out, lv)
	}
	return out, rows.Err()
}

// Backlinks returns inbound links pointing at a page.
func (db *DB) Backlinks(toPageID int64) ([]Backlink, error) {
	rows, err := db.Query(
		`SELECT p.slug, l.raw, l.status
		   FROM links l JOIN pages p ON p.id = l.from_page_id
		  WHERE l.to_page_id = ? AND p.deleted_at IS NULL ORDER BY l.id`, toPageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Backlink{}
	for rows.Next() {
		var b Backlink
		if err := rows.Scan(&b.FromSlug, &b.Raw, &b.Status); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// MarkLinksToPageMissing clears resolution for all links pointing at a page that
// has gone away (e.g. after delete), setting them missing.
func (db *DB) MarkLinksToPageMissing(toPageID int64) error {
	_, err := db.Exec(
		`UPDATE links SET to_page_id = NULL, status = ? WHERE to_page_id = ?`,
		StatusMissing, toPageID)
	return err
}

// PageLinkCounts is a per-page summary of its graph position.
type PageLinkCounts struct {
	PageID    int64 `json:"-"`
	Outgoing  int   `json:"outgoing"`  // resolved outgoing links
	Backlinks int   `json:"backlinks"` // resolved inbound links
	Missing   int   `json:"missing"`   // outgoing missing links
	Ambiguous int   `json:"ambiguous"` // outgoing ambiguous links
}

// PageCountsInWiki returns link counts for every live page in a wiki, keyed by
// page id. Computed in a few aggregate queries rather than per-page.
func (db *DB) PageCountsInWiki(wikiID int64) (map[int64]*PageLinkCounts, error) {
	out := map[int64]*PageLinkCounts{}
	get := func(id int64) *PageLinkCounts {
		if c, ok := out[id]; ok {
			return c
		}
		c := &PageLinkCounts{PageID: id}
		out[id] = c
		return c
	}

	// Outgoing links grouped by source page + status.
	rows, err := db.Query(
		`SELECT from_page_id, status, COUNT(*) FROM links WHERE wiki_id = ? GROUP BY from_page_id, status`,
		wikiID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var status string
		var n int
		if err := rows.Scan(&id, &status, &n); err != nil {
			rows.Close()
			return nil, err
		}
		c := get(id)
		switch status {
		case StatusResolved:
			c.Outgoing += n
		case StatusMissing:
			c.Missing += n
		case StatusAmbiguous:
			c.Ambiguous += n
		}
	}
	rows.Close()

	// Resolved backlinks grouped by target page.
	rows2, err := db.Query(
		`SELECT to_page_id, COUNT(*) FROM links
		   WHERE wiki_id = ? AND to_page_id IS NOT NULL AND status = ? GROUP BY to_page_id`,
		wikiID, StatusResolved)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var id int64
		var n int
		if err := rows2.Scan(&id, &n); err != nil {
			return nil, err
		}
		get(id).Backlinks = n
	}
	return out, rows2.Err()
}

// BrokenLink is an unresolved (missing/ambiguous) cross-reference, with its
// source page, for the verification report.
type BrokenLink struct {
	FromSlug    string `json:"from_slug"`
	FromRelPath string `json:"from_rel_path"`
	Raw         string `json:"raw"`
	Target      string `json:"target"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
}

// BrokenLinks returns every missing/ambiguous outgoing link in a wiki, joined to
// its source page, ordered by source path.
func (db *DB) BrokenLinks(wikiID int64) ([]BrokenLink, error) {
	rows, err := db.Query(
		`SELECT p.slug, p.rel_path, l.raw, l.target, l.kind, l.status
		   FROM links l JOIN pages p ON p.id = l.from_page_id
		  WHERE l.wiki_id = ? AND l.status != ? AND p.deleted_at IS NULL
		  ORDER BY p.rel_path, l.id`,
		wikiID, StatusResolved)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BrokenLink{}
	for rows.Next() {
		var b BrokenLink
		if err := rows.Scan(&b.FromSlug, &b.FromRelPath, &b.Raw, &b.Target, &b.Kind, &b.Status); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// WikiStats is a wiki-level graph health summary.
type WikiStats struct {
	Pages     int `json:"pages"`
	Resolved  int `json:"resolved"`
	Missing   int `json:"missing"`
	Ambiguous int `json:"ambiguous"`
}

// Stats returns graph health for a wiki.
func (db *DB) Stats(wikiID int64) (*WikiStats, error) {
	var s WikiStats
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM pages WHERE wiki_id = ? AND deleted_at IS NULL`, wikiID,
	).Scan(&s.Pages); err != nil {
		return nil, err
	}
	rows, err := db.Query(
		`SELECT status, COUNT(*) FROM links WHERE wiki_id = ? GROUP BY status`, wikiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		switch status {
		case StatusResolved:
			s.Resolved = n
		case StatusMissing:
			s.Missing = n
		case StatusAmbiguous:
			s.Ambiguous = n
		}
	}
	return &s, rows.Err()
}

// FromPageIDsInWiki returns the ids of live pages in a wiki (used to re-resolve
// all links after a sync).
func (db *DB) FromPageIDsInWiki(wikiID int64) ([]int64, error) {
	rows, err := db.Query(
		`SELECT id FROM pages WHERE wiki_id = ? AND deleted_at IS NULL`, wikiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
