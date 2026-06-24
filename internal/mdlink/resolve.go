package mdlink

import (
	"path"
	"strings"

	"github.com/plato/plato/internal/store"
)

// Resolution is the outcome of resolving a Ref against a wiki's pages.
type Resolution struct {
	Ref      Ref
	Status   string // store.StatusResolved | Missing | Ambiguous
	ToPageID int64  // 0 when unresolved
}

// Resolver provides the page lookups needed to resolve links within one wiki.
// store.DB satisfies this.
type Resolver interface {
	PageBySlug(wikiID int64, slug string) (*store.Page, error)
	PagesByTitle(wikiID int64, title string, caseInsensitive bool) ([]store.Page, error)
	PageByRelPath(wikiID int64, relPath string) (*store.Page, error)
}

// Resolve resolves one reference for a page located at fromRelPath in wikiID.
func Resolve(r Resolver, wikiID int64, fromRelPath string, ref Ref) (Resolution, error) {
	switch ref.Kind {
	case KindWiki:
		return resolveWiki(r, wikiID, ref)
	case KindRelative:
		return resolveRelative(r, wikiID, fromRelPath, ref)
	default:
		return Resolution{Ref: ref, Status: store.StatusMissing}, nil
	}
}

// resolveWiki: exact slug -> unique exact title -> unique ci title -> missing;
// multiple title matches -> ambiguous.
func resolveWiki(r Resolver, wikiID int64, ref Ref) (Resolution, error) {
	if p, err := r.PageBySlug(wikiID, ref.Target); err != nil {
		return Resolution{}, err
	} else if p != nil {
		return Resolution{ref, store.StatusResolved, p.ID}, nil
	}

	exact, err := r.PagesByTitle(wikiID, ref.Target, false)
	if err != nil {
		return Resolution{}, err
	}
	switch {
	case len(exact) == 1:
		return Resolution{ref, store.StatusResolved, exact[0].ID}, nil
	case len(exact) > 1:
		return Resolution{ref, store.StatusAmbiguous, 0}, nil
	}

	ci, err := r.PagesByTitle(wikiID, ref.Target, true)
	if err != nil {
		return Resolution{}, err
	}
	switch {
	case len(ci) == 1:
		return Resolution{ref, store.StatusResolved, ci[0].ID}, nil
	case len(ci) > 1:
		return Resolution{ref, store.StatusAmbiguous, 0}, nil
	}

	return Resolution{ref, store.StatusMissing, 0}, nil
}

// resolveRelative: resolve the dest against the source page's directory, normalize,
// match exact rel_path. No title fallback.
func resolveRelative(r Resolver, wikiID int64, fromRelPath string, ref Ref) (Resolution, error) {
	dest := ref.Target
	if i := strings.IndexByte(dest, '#'); i >= 0 {
		dest = dest[:i]
	}
	joined := path.Join(path.Dir(fromRelPath), dest)
	joined = path.Clean(joined)
	// A link escaping the wiki root can never match an indexed page.
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return Resolution{ref, store.StatusMissing, 0}, nil
	}
	p, err := r.PageByRelPath(wikiID, joined)
	if err != nil {
		return Resolution{}, err
	}
	if p != nil && !p.DeletedAt.Valid {
		return Resolution{ref, store.StatusResolved, p.ID}, nil
	}
	return Resolution{ref, store.StatusMissing, 0}, nil
}
