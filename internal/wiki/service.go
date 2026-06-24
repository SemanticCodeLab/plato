package wiki

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/plato/plato/internal/gitsrc"
	"github.com/plato/plato/internal/mdlink"
	"github.com/plato/plato/internal/store"
)

// Errors surfaced to callers (mapped to HTTP status by the API layer).
var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrExists    = errors.New("already exists")
	ErrBadInput  = errors.New("bad input")
)

// Service is the wiki domain layer over the store and filesystem.
type Service struct {
	DB      *store.DB
	WikiDir string // root directory containing per-wiki subdirectories
}

// New creates a Service.
func New(db *store.DB, wikiDir string) *Service {
	return &Service{DB: db, WikiDir: wikiDir}
}

// PageWithContent is a page plus its file content, for API responses.
type PageWithContent struct {
	store.Page
	Content string `json:"content"`
}

// CreateWiki creates an empty project (wiki) row and its directory.
func (s *Service) CreateWiki(slug, title string) (*store.Wiki, error) {
	return s.createWikiRow(slug, title, store.SourceEmpty, "", "", "")
}

// createWikiRow is the shared project-row + directory creator.
func (s *Service) createWikiRow(slug, title, sourceType, sourceURL, branch, subdir string) (*store.Wiki, error) {
	slug = Slugify(slug)
	if title == "" {
		title = slug
	}
	if existing, err := s.DB.WikiBySlug(slug); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrExists
	}
	root := filepath.Join(s.WikiDir, slug)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	if _, err := s.DB.CreateWiki(slug, title, root, sourceType, sourceURL, branch, subdir); err != nil {
		return nil, err
	}
	return s.DB.WikiBySlug(slug)
}

// CreateEmptyProject creates an empty project and seeds a starter Home.md.
func (s *Service) CreateEmptyProject(slug, title string) (*store.Wiki, error) {
	w, err := s.CreateWiki(slug, title)
	if err != nil {
		return nil, err
	}
	starter := "# " + w.Title + "\n\nWelcome to your new Plato project.\n"
	if _, err := s.CreatePage(w, w.Title, "Home.md", starter); err != nil {
		return nil, err
	}
	_ = s.DB.SetLastIndexed(w.ID)
	return s.DB.WikiBySlug(w.Slug)
}

// CreateLocalProject creates a project and imports Markdown from a local folder.
func (s *Service) CreateLocalProject(slug, title, localPath string) (*store.Wiki, error) {
	if fi, err := os.Stat(localPath); err != nil || !fi.IsDir() {
		return nil, ErrBadInput
	}
	w, err := s.createWikiRow(slug, title, store.SourceLocal, localPath, "", "")
	if err != nil {
		return nil, err
	}
	if err := s.ImportDir(w, localPath); err != nil {
		return nil, err
	}
	_ = s.DB.SetLastIndexed(w.ID)
	return s.DB.WikiBySlug(w.Slug)
}

// CreateGitProject validates+clones a public HTTPS repo, imports its Markdown
// (optionally from a subdir), and records the git source metadata. The clone
// lives under <wiki-dir>/.repos/<slug>; pages are copied into the project dir so
// the invariant "pages live under <wiki-dir>/<slug>" always holds.
func (s *Service) CreateGitProject(slug, title, url, branch, subdir string) (*store.Wiki, error) {
	if err := gitsrc.ValidateURL(url); err != nil {
		return nil, err
	}
	cleanSlug := Slugify(slug)
	srcRoot, err := gitsrc.Clone(s.WikiDir, cleanSlug, url, branch, subdir)
	if err != nil {
		return nil, err
	}
	w, err := s.createWikiRow(slug, title, store.SourceGit, url, branch, subdir)
	if err != nil {
		return nil, err
	}
	if err := s.ImportDir(w, srcRoot); err != nil {
		return nil, err
	}
	_ = s.DB.SetLastIndexed(w.ID)
	return s.DB.WikiBySlug(w.Slug)
}

// GitPull pulls latest for a git-backed project, re-imports Markdown, and
// reindexes. Returns counts of pages changed and links reindexed.
func (s *Service) GitPull(w *store.Wiki) (pagesChanged, linksReindexed int, err error) {
	if w.SourceType != store.SourceGit {
		return 0, 0, ErrBadInput
	}
	srcRoot, err := gitsrc.Pull(s.WikiDir, w.Slug, w.SourceBranch.String, w.SourceSubdir.String)
	if err != nil {
		return 0, 0, err
	}
	before, _ := s.DB.ListPages(w.ID)
	beforeHash := map[string]string{}
	for _, p := range before {
		beforeHash[p.RelPath] = p.ContentHash
	}
	if err := s.ImportDir(w, srcRoot); err != nil {
		return 0, 0, err
	}
	after, _ := s.DB.ListPages(w.ID)
	for _, p := range after {
		if beforeHash[p.RelPath] != p.ContentHash {
			pagesChanged++
		}
	}
	// Count total resolved+unresolved outgoing links as a rough reindex tally.
	for _, p := range after {
		links, _ := s.DB.OutgoingLinks(p.ID)
		linksReindexed += len(links)
	}
	_ = s.DB.SetLastIndexed(w.ID)
	return pagesChanged, linksReindexed, nil
}

// GetPage loads a page and its file content by slug.
func (s *Service) GetPage(w *store.Wiki, slug string) (*PageWithContent, error) {
	p, err := s.DB.PageBySlug(w.ID, slug)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrNotFound
	}
	abs, err := AbsPath(w.RootPath, p.RelPath)
	if err != nil {
		return nil, err
	}
	content, _, err := ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return &PageWithContent{Page: *p, Content: string(content)}, nil
}

// CreatePage validates the path, writes the file, indexes the page, and reindexes links.
func (s *Service) CreatePage(w *store.Wiki, title, relPath, content string) (*store.Page, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return nil, err
	}
	if existing, err := s.DB.PageByRelPath(w.ID, clean); err != nil {
		return nil, err
	} else if existing != nil && !existing.DeletedAt.Valid {
		return nil, ErrExists
	}
	if title == "" {
		title = TitleFromContentOrPath(content, clean)
	}
	abs, err := AbsPath(w.RootPath, clean)
	if err != nil {
		return nil, err
	}
	hash, err := WriteFileAtomic(abs, []byte(content))
	if err != nil {
		return nil, err
	}
	slug, err := UniqueSlug(title, func(c string) (bool, error) {
		return s.DB.SlugExists(w.ID, c, 0)
	})
	if err != nil {
		return nil, err
	}

	var p *store.Page
	if existing, _ := s.DB.PageByRelPath(w.ID, clean); existing != nil {
		// Resurrect a previously soft-deleted page at the same rel_path.
		if err := s.DB.UpdatePage(existing.ID, slug, title, hash); err != nil {
			return nil, err
		}
		p, err = s.DB.PageByID(existing.ID)
	} else {
		p, err = s.DB.CreatePage(w.ID, slug, title, clean, hash)
	}
	if err != nil {
		return nil, err
	}
	if err := s.ReindexPage(w, p, content); err != nil {
		return nil, err
	}
	s.ResolveWikiLinks(w) // re-resolve so previously-missing links can now resolve
	return p, nil
}

// UpdatePage applies optimistic concurrency: baseHash must equal the current
// content hash, else ErrConflict (with the current hash via ConflictHash).
func (s *Service) UpdatePage(w *store.Wiki, slug, content, baseHash string) (*store.Page, error) {
	p, err := s.DB.PageBySlug(w.ID, slug)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, ErrNotFound
	}
	abs, err := AbsPath(w.RootPath, p.RelPath)
	if err != nil {
		return nil, err
	}
	// Current on-disk hash is the source of truth for conflict detection.
	_, currentHash, err := ReadFile(abs)
	if err != nil {
		return nil, err
	}
	if baseHash != "" && baseHash != currentHash {
		return nil, &ConflictError{CurrentHash: currentHash}
	}
	hash, err := WriteFileAtomic(abs, []byte(content))
	if err != nil {
		return nil, err
	}
	title := TitleFromContentOrPath(content, p.RelPath)
	// Keep slug stable on update (slug only changes via sync re-derivation).
	if err := s.DB.UpdatePage(p.ID, p.Slug, title, hash); err != nil {
		return nil, err
	}
	p, _ = s.DB.PageByID(p.ID)
	if err := s.ReindexPage(w, p, content); err != nil {
		return nil, err
	}
	return p, nil
}

// DeletePage soft-deletes the row, removes the file, and marks inbound links missing.
func (s *Service) DeletePage(w *store.Wiki, slug string) error {
	p, err := s.DB.PageBySlug(w.ID, slug)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrNotFound
	}
	abs, err := AbsPath(w.RootPath, p.RelPath)
	if err != nil {
		return err
	}
	_ = os.Remove(abs)
	if err := s.DB.DeleteLinksFrom(p.ID); err != nil {
		return err
	}
	if err := s.DB.SoftDeletePage(p.ID); err != nil {
		return err
	}
	return s.DB.MarkLinksToPageMissing(p.ID)
}

// ImportDir walks all *.md under srcDir and upserts them into the project by
// rel_path, then re-resolves the wiki's links. It does not delete pages whose
// source vanished (callers that need that should use the sync package).
func (s *Service) ImportDir(w *store.Wiki, srcDir string) error {
	err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if _, _, err := s.SyncPage(w, filepath.ToSlash(rel), string(content)); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.ResolveWikiLinks(w)
}

// SyncResult reports what happened to one page during a sync.
type SyncResult struct {
	RelPath string
	Slug    string
	Action  string // created | updated | unchanged
}

// SyncPage upserts a page by rel_path: writes the file into the wiki dir, creates
// or updates the index row (only touching metadata when the content hash changed),
// and reindexes its links. Returns the page and the action taken.
func (s *Service) SyncPage(w *store.Wiki, relPath, content string) (*store.Page, string, error) {
	clean, err := CleanRelPath(relPath)
	if err != nil {
		return nil, "", err
	}
	abs, err := AbsPath(w.RootPath, clean)
	if err != nil {
		return nil, "", err
	}
	hash, err := WriteFileAtomic(abs, []byte(content))
	if err != nil {
		return nil, "", err
	}
	title := TitleFromContentOrPath(content, clean)

	existing, err := s.DB.PageByRelPath(w.ID, clean)
	if err != nil {
		return nil, "", err
	}
	if existing == nil {
		slug, err := UniqueSlug(title, func(c string) (bool, error) {
			return s.DB.SlugExists(w.ID, c, 0)
		})
		if err != nil {
			return nil, "", err
		}
		p, err := s.DB.CreatePage(w.ID, slug, title, clean, hash)
		if err != nil {
			return nil, "", err
		}
		return p, "created", s.reindexAfterSync(w, p, content)
	}

	// Existing (possibly soft-deleted) page at this rel_path.
	wasDeleted := existing.DeletedAt.Valid
	if !wasDeleted && existing.ContentHash == hash && existing.Title == title {
		return existing, "unchanged", nil
	}
	slug := existing.Slug
	if wasDeleted || existing.Title != title {
		slug, err = UniqueSlug(title, func(c string) (bool, error) {
			return s.DB.SlugExists(w.ID, c, existing.ID)
		})
		if err != nil {
			return nil, "", err
		}
	}
	if err := s.DB.UpdatePage(existing.ID, slug, title, hash); err != nil {
		return nil, "", err
	}
	p, _ := s.DB.PageByID(existing.ID)
	action := "updated"
	if wasDeleted {
		action = "created"
	}
	return p, action, s.reindexAfterSync(w, p, content)
}

func (s *Service) reindexAfterSync(w *store.Wiki, p *store.Page, content string) error {
	return s.ReindexPage(w, p, content)
}

// LivePageRelPaths returns the set of rel_paths for live pages in a wiki.
func (s *Service) LivePageRelPaths(w *store.Wiki) (map[string]*store.Page, error) {
	pages, err := s.DB.ListPages(w.ID)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*store.Page, len(pages))
	for i := range pages {
		m[pages[i].RelPath] = &pages[i]
	}
	return m, nil
}

// PageBrokenLinks returns the missing/ambiguous outgoing links for one page,
// derived from the given content without persisting (used for strict-mode
// validation before a write is committed).
func (s *Service) PageBrokenLinks(w *store.Wiki, fromRelPath, content string) ([]mdlink.Resolution, error) {
	var broken []mdlink.Resolution
	for _, ref := range mdlink.Parse(content) {
		res, err := mdlink.Resolve(s.DB, w.ID, fromRelPath, ref)
		if err != nil {
			return nil, err
		}
		if res.Status != store.StatusResolved {
			broken = append(broken, res)
		}
	}
	return broken, nil
}

// ReindexPage parses content's links and rewrites this page's outgoing link rows.
func (s *Service) ReindexPage(w *store.Wiki, p *store.Page, content string) error {
	if err := s.DB.DeleteLinksFrom(p.ID); err != nil {
		return err
	}
	for _, ref := range mdlink.Parse(content) {
		res, err := mdlink.Resolve(s.DB, w.ID, p.RelPath, ref)
		if err != nil {
			return err
		}
		l := store.Link{
			WikiID:     w.ID,
			FromPageID: p.ID,
			Target:     ref.Target,
			Raw:        ref.Raw,
			Kind:       ref.Kind,
			Status:     res.Status,
		}
		if ref.Label != "" {
			l.Label.String, l.Label.Valid = ref.Label, true
		}
		if res.ToPageID != 0 {
			l.ToPageID.Int64, l.ToPageID.Valid = res.ToPageID, true
		}
		if err := s.DB.InsertLink(l); err != nil {
			return err
		}
	}
	return nil
}

// ResolveWikiLinks re-resolves every link in a wiki by reindexing each live page
// from its file. Used after bulk changes (sync) so cross-references settle.
func (s *Service) ResolveWikiLinks(w *store.Wiki) error {
	pages, err := s.DB.ListPages(w.ID)
	if err != nil {
		return err
	}
	for i := range pages {
		abs, err := AbsPath(w.RootPath, pages[i].RelPath)
		if err != nil {
			continue
		}
		content, _, err := ReadFile(abs)
		if err != nil {
			continue
		}
		if err := s.ReindexPage(w, &pages[i], string(content)); err != nil {
			return err
		}
	}
	return nil
}

// TitleFromContentOrPath returns the first H1 in content, else the filename stem.
func TitleFromContentOrPath(content, relPath string) string {
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(t[2:])
		}
	}
	base := filepath.Base(relPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ConflictError carries the current content hash for a 409 response.
type ConflictError struct{ CurrentHash string }

func (e *ConflictError) Error() string { return "conflict" }
