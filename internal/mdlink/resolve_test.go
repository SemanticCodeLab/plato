package mdlink

import (
	"testing"

	"github.com/plato/plato/internal/store"
)

// fakeResolver implements Resolver over in-memory pages.
type fakeResolver struct {
	bySlug    map[string]*store.Page
	byTitle   map[string][]store.Page // exact
	byRelPath map[string]*store.Page
}

func (f *fakeResolver) PageBySlug(_ int64, slug string) (*store.Page, error) {
	return f.bySlug[slug], nil
}
func (f *fakeResolver) PagesByTitle(_ int64, title string, ci bool) ([]store.Page, error) {
	if !ci {
		return f.byTitle[title], nil
	}
	var out []store.Page
	for k, v := range f.byTitle {
		if equalFold(k, title) {
			out = append(out, v...)
		}
	}
	return out, nil
}
func (f *fakeResolver) PageByRelPath(_ int64, rel string) (*store.Page, error) {
	return f.byRelPath[rel], nil
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 32
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func TestResolveWiki(t *testing.T) {
	r := &fakeResolver{
		bySlug: map[string]*store.Page{
			"database": {ID: 1, Slug: "database", Title: "Database"},
		},
		byTitle: map[string][]store.Page{
			"Database": {{ID: 1, Slug: "database", Title: "Database"}},
			"Dup":      {{ID: 2}, {ID: 3}},
		},
	}

	// by slug
	res, _ := Resolve(r, 1, "a.md", Ref{Kind: KindWiki, Target: "database"})
	if res.Status != store.StatusResolved || res.ToPageID != 1 {
		t.Errorf("slug resolve: %+v", res)
	}
	// by exact title
	res, _ = Resolve(r, 1, "a.md", Ref{Kind: KindWiki, Target: "Database"})
	if res.Status != store.StatusResolved {
		t.Errorf("title resolve: %+v", res)
	}
	// case-insensitive title
	res, _ = Resolve(r, 1, "a.md", Ref{Kind: KindWiki, Target: "DATABASE"})
	if res.Status != store.StatusResolved {
		t.Errorf("ci title resolve: %+v", res)
	}
	// missing
	res, _ = Resolve(r, 1, "a.md", Ref{Kind: KindWiki, Target: "Nope"})
	if res.Status != store.StatusMissing {
		t.Errorf("missing: %+v", res)
	}
	// ambiguous
	res, _ = Resolve(r, 1, "a.md", Ref{Kind: KindWiki, Target: "Dup"})
	if res.Status != store.StatusAmbiguous {
		t.Errorf("ambiguous: %+v", res)
	}
}

func TestResolveRelative(t *testing.T) {
	r := &fakeResolver{
		byRelPath: map[string]*store.Page{
			"docs/db.md": {ID: 5, RelPath: "docs/db.md"},
		},
	}
	// resolved relative to source dir docs/
	res, _ := Resolve(r, 1, "docs/auth.md", Ref{Kind: KindRelative, Target: "./db.md"})
	if res.Status != store.StatusResolved || res.ToPageID != 5 {
		t.Errorf("relative resolve: %+v", res)
	}
	// missing relative
	res, _ = Resolve(r, 1, "docs/auth.md", Ref{Kind: KindRelative, Target: "./nope.md"})
	if res.Status != store.StatusMissing {
		t.Errorf("relative missing: %+v", res)
	}
}
