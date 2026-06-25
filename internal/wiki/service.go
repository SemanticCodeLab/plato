package wiki

import (
	"errors"
	"io/fs"
	"os"
	"path"
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

// relatedHeading is the managed section appended links are placed under.
const relatedHeading = "## Related"

// AddLinkSpec describes a link to add. Exactly one target form is used, in
// priority order: Slug, RelPath, Title.
type AddLinkSpec struct {
	ToSlug  string
	ToPath  string
	ToTitle string
	Label   string
}

// AddForwardLink adds a link from page `fromSlug` to the target, by writing it
// into the source page's Markdown (under a managed "## Related" section) and
// marking the resulting link manual. Returns the updated page.
//
// Adding a backlink "into B from A" is the same operation with fromSlug=A,
// target=B — backlinks remain a derived view of forward links.
func (s *Service) AddForwardLink(w *store.Wiki, fromSlug string, spec AddLinkSpec) (*store.Page, error) {
	from, err := s.DB.PageBySlug(w.ID, fromSlug)
	if err != nil {
		return nil, err
	}
	if from == nil {
		return nil, ErrNotFound
	}
	linkText, kind, target, err := s.renderLink(w, from, spec)
	if err != nil {
		return nil, err
	}

	abs, err := AbsPath(w.RootPath, from.RelPath)
	if err != nil {
		return nil, err
	}
	content, _, err := ReadFile(abs)
	if err != nil {
		return nil, err
	}
	newContent := appendRelated(string(content), linkText)
	hash, err := WriteFileAtomic(abs, []byte(newContent))
	if err != nil {
		return nil, err
	}
	if err := s.DB.UpdatePage(from.ID, from.Slug, from.Title, hash); err != nil {
		return nil, err
	}
	// Reindex, then mark this specific link manual so it survives future reindexes.
	from, _ = s.DB.PageByID(from.ID)
	if err := s.ReindexPage(w, from, newContent); err != nil {
		return nil, err
	}
	if err := s.DB.MarkLinkManual(from.ID, kind, target); err != nil {
		return nil, err
	}
	return from, nil
}

// RemoveForwardLink removes a previously added link bullet (matching linkText)
// from the source page's "## Related" section and reindexes. linkText is the raw
// markdown of the link (e.g. "[[database]]" or "[label](./x.md)").
func (s *Service) RemoveForwardLink(w *store.Wiki, fromSlug, linkText string) (*store.Page, error) {
	from, err := s.DB.PageBySlug(w.ID, fromSlug)
	if err != nil {
		return nil, err
	}
	if from == nil {
		return nil, ErrNotFound
	}
	abs, err := AbsPath(w.RootPath, from.RelPath)
	if err != nil {
		return nil, err
	}
	content, _, err := ReadFile(abs)
	if err != nil {
		return nil, err
	}
	newContent := removeRelatedBullet(string(content), linkText)
	if newContent == string(content) {
		return nil, ErrNotFound // nothing removed
	}
	hash, err := WriteFileAtomic(abs, []byte(newContent))
	if err != nil {
		return nil, err
	}
	if err := s.DB.UpdatePage(from.ID, from.Slug, from.Title, hash); err != nil {
		return nil, err
	}
	from, _ = s.DB.PageByID(from.ID)
	if err := s.ReindexPage(w, from, newContent); err != nil {
		return nil, err
	}
	return from, nil
}

// removeRelatedBullet deletes the "- <linkText>" line if present.
func removeRelatedBullet(content, linkText string) string {
	bullet := "- " + linkText
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if strings.TrimSpace(ln) == bullet {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// renderLink turns an AddLinkSpec into Markdown link text plus its (kind,target)
// as they will appear in the link index.
func (s *Service) renderLink(w *store.Wiki, from *store.Page, spec AddLinkSpec) (text, kind, target string, err error) {
	switch {
	case spec.ToTitle != "":
		// Wikilink by title.
		if spec.Label != "" {
			return "[[" + spec.ToTitle + "|" + spec.Label + "]]", store.KindWiki, spec.ToTitle, nil
		}
		return "[[" + spec.ToTitle + "]]", store.KindWiki, spec.ToTitle, nil
	case spec.ToPath != "":
		clean, err := CleanRelPath(spec.ToPath)
		if err != nil {
			return "", "", "", err
		}
		rel := relPathFrom(from.RelPath, clean)
		label := spec.Label
		if label == "" {
			label = clean
		}
		return "[" + label + "](" + rel + ")", store.KindRelative, rel, nil
	case spec.ToSlug != "":
		// Resolve slug to a page; emit a wikilink by slug (stable identifier).
		tp, err := s.DB.PageBySlug(w.ID, spec.ToSlug)
		if err != nil {
			return "", "", "", err
		}
		if tp == nil {
			return "", "", "", ErrNotFound
		}
		if spec.Label != "" {
			return "[[" + tp.Slug + "|" + spec.Label + "]]", store.KindWiki, tp.Slug, nil
		}
		return "[[" + tp.Slug + "]]", store.KindWiki, tp.Slug, nil
	default:
		return "", "", "", ErrBadInput
	}
}

// appendRelated adds linkText as a bullet under a managed "## Related" section,
// creating the section if absent and avoiding duplicate bullets.
func appendRelated(content, linkText string) string {
	bullet := "- " + linkText
	if strings.Contains(content, bullet) {
		return content // already present
	}
	trimmed := strings.TrimRight(content, "\n")
	if strings.Contains(content, relatedHeading) {
		return trimmed + "\n" + bullet + "\n"
	}
	return trimmed + "\n\n" + relatedHeading + "\n" + bullet + "\n"
}

// relPathFrom computes a relative link path from a source page to a target path,
// operating on slash-separated rel_paths.
func relPathFrom(fromRel, toRel string) string {
	fromDir := path.Dir(fromRel)
	if fromDir == "." {
		fromDir = ""
	}
	rel := pathRel(fromDir, toRel)
	// Ensure a leading "./" for same-dir links so it reads as relative.
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}

// pathRel returns toRel expressed relative to baseDir (both slash-separated,
// project-root-relative). Falls back to toRel if it cannot compute.
func pathRel(baseDir, toRel string) string {
	if baseDir == "" {
		return toRel
	}
	baseParts := strings.Split(baseDir, "/")
	toParts := strings.Split(toRel, "/")
	// drop common prefix
	i := 0
	for i < len(baseParts) && i < len(toParts)-1 && baseParts[i] == toParts[i] {
		i++
	}
	var out []string
	for j := i; j < len(baseParts); j++ {
		out = append(out, "..")
	}
	out = append(out, toParts[i:]...)
	return strings.Join(out, "/")
}

// ReindexPage parses content's links and rewrites this page's outgoing link rows.
// Links previously marked manual are re-tagged manual when rediscovered, so the
// auto/manual origin survives reindexing.
func (s *Service) ReindexPage(w *store.Wiki, p *store.Page, content string) error {
	manual, err := s.DB.ManualLinkKeys(p.ID)
	if err != nil {
		return err
	}
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
			Origin:     store.OriginAuto,
		}
		if manual[ref.Kind+"|"+ref.Target] {
			l.Origin = store.OriginManual
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
