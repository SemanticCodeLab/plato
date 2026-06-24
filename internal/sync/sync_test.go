package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/plato/plato/internal/store"
	"github.com/plato/plato/internal/wiki"
)

func setup(t *testing.T) (*wiki.Service, *store.Wiki, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := wiki.New(db, t.TempDir())
	w, err := svc.CreateWiki("demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	return svc, w, src
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncImportRerunDelete(t *testing.T) {
	svc, w, src := setup(t)

	write(t, src, "A.md", "# A\n\nlinks to [[B]] and [rel](./B.md)\n")
	write(t, src, "B.md", "# B\n\nback to [[A]]\n")

	rep, err := Run(svc, w, src, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) != 2 {
		t.Fatalf("created = %d, want 2", len(rep.Created))
	}

	pages, _ := svc.DB.ListPages(w.ID)
	if len(pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(pages))
	}

	// All links should be resolved after sync re-resolution.
	for _, p := range pages {
		links, _ := svc.DB.OutgoingLinks(p.ID)
		for _, l := range links {
			if l.Status != store.StatusResolved {
				t.Errorf("page %s link %q status %s, want resolved", p.Slug, l.Raw, l.Status)
			}
		}
	}

	// Rerun without changes -> no new pages, all unchanged.
	rep2, err := Run(svc, w, src, false, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep2.Created) != 0 {
		t.Errorf("rerun created = %d, want 0", len(rep2.Created))
	}
	pages, _ = svc.DB.ListPages(w.ID)
	if len(pages) != 2 {
		t.Errorf("after rerun pages = %d, want 2 (no duplicates)", len(pages))
	}

	// Remove B, sync with --delete -> B soft-deleted, links to B become missing.
	if err := os.Remove(filepath.Join(src, "B.md")); err != nil {
		t.Fatal(err)
	}
	rep3, err := Run(svc, w, src, true, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(rep3.Deleted) != 1 {
		t.Errorf("deleted = %d, want 1", len(rep3.Deleted))
	}
	pages, _ = svc.DB.ListPages(w.ID)
	if len(pages) != 1 {
		t.Errorf("after delete pages = %d, want 1", len(pages))
	}
	// A's link to B should now be missing.
	a, _ := svc.DB.PageBySlug(w.ID, "a")
	if a == nil {
		t.Fatal("page A missing")
	}
	links, _ := svc.DB.OutgoingLinks(a.ID)
	var sawMissing bool
	for _, l := range links {
		if l.Status == store.StatusMissing {
			sawMissing = true
		}
	}
	if !sawMissing {
		t.Errorf("expected a missing link to deleted B, got %+v", links)
	}
}
