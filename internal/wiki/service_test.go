package wiki

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plato/plato/internal/store"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db, t.TempDir())
}

func TestConcurrencyConflict(t *testing.T) {
	s := newSvc(t)
	w, err := s.CreateWiki("demo", "Demo")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePage(w, "Auth", "Auth.md", "# Auth\nv1\n")
	if err != nil {
		t.Fatal(err)
	}

	// Stale base hash -> conflict.
	_, err = s.UpdatePage(w, p.Slug, "# Auth\nv2\n", "sha256:stale")
	var conflict *ConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected ConflictError, got %v", err)
	}
	if conflict.CurrentHash == "" {
		t.Error("conflict should carry current hash")
	}

	// Correct base hash -> success.
	pwc, err := s.GetPage(w, p.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdatePage(w, p.Slug, "# Auth\nv2\n", pwc.ContentHash); err != nil {
		t.Fatalf("matching base_hash should succeed: %v", err)
	}
}

func TestCreateAndResolveLinks(t *testing.T) {
	s := newSvc(t)
	w, _ := s.CreateWiki("demo", "Demo")

	// Auth links to Database (exists) and Missing (does not).
	_, err := s.CreatePage(w, "Database", "Database.md", "# Database\n")
	if err != nil {
		t.Fatal(err)
	}
	auth, err := s.CreatePage(w, "Authentication", "Authentication.md",
		"# Authentication\n\n[[Database]] and [[Missing]] and [rel](./Database.md)\n")
	if err != nil {
		t.Fatal(err)
	}

	links, err := s.DB.OutgoingLinks(auth.ID)
	if err != nil {
		t.Fatal(err)
	}
	var resolved, missing int
	for _, l := range links {
		switch l.Status {
		case store.StatusResolved:
			resolved++
		case store.StatusMissing:
			missing++
		}
	}
	if resolved != 2 { // [[Database]] + ./Database.md
		t.Errorf("resolved = %d, want 2 (%+v)", resolved, links)
	}
	if missing != 1 { // [[Missing]]
		t.Errorf("missing = %d, want 1 (%+v)", missing, links)
	}
}

func TestPageBrokenLinks(t *testing.T) {
	s := newSvc(t)
	w, _ := s.CreateWiki("demo", "Demo")
	_, _ = s.CreatePage(w, "Database", "Database.md", "# Database\n")

	// One resolved ([[Database]]) + one broken ([[Ghost]]).
	broken, err := s.PageBrokenLinks(w, "New.md", "# New\n[[Database]] and [[Ghost]]\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 1 {
		t.Fatalf("broken = %d, want 1: %+v", len(broken), broken)
	}
	if broken[0].Ref.Target != "Ghost" {
		t.Errorf("expected Ghost broken, got %q", broken[0].Ref.Target)
	}

	// All resolved -> none broken.
	broken, _ = s.PageBrokenLinks(w, "New.md", "# New\n[[Database]]\n")
	if len(broken) != 0 {
		t.Errorf("expected 0 broken, got %d", len(broken))
	}
}

func TestAddForwardLinkAndOriginSurvivesReindex(t *testing.T) {
	s := newSvc(t)
	w, _ := s.CreateWiki("demo", "Demo")
	_, _ = s.CreatePage(w, "Database", "Database.md", "# Database\n")
	from, _ := s.CreatePage(w, "Auth", "Auth.md", "# Auth\n")

	// Add an explicit forward link Auth -> Database by slug.
	if _, err := s.AddForwardLink(w, from.Slug, AddLinkSpec{ToSlug: "database"}); err != nil {
		t.Fatal(err)
	}
	links, _ := s.DB.OutgoingLinks(from.ID)
	if len(links) != 1 || links[0].Status != store.StatusResolved {
		t.Fatalf("expected 1 resolved link, got %+v", links)
	}
	if links[0].Origin != store.OriginManual {
		t.Errorf("expected manual origin, got %q", links[0].Origin)
	}

	// File should contain the managed Related section.
	pwc, _ := s.GetPage(w, from.Slug)
	if !strings.Contains(pwc.Content, "## Related") || !strings.Contains(pwc.Content, "[[database]]") {
		t.Errorf("link not written into file:\n%s", pwc.Content)
	}

	// Reindex from file — manual origin must survive.
	_ = s.ResolveWikiLinks(w)
	links, _ = s.DB.OutgoingLinks(from.ID)
	if len(links) != 1 || links[0].Origin != store.OriginManual {
		t.Errorf("manual origin lost after reindex: %+v", links)
	}

	// Remove it.
	if _, err := s.RemoveForwardLink(w, from.Slug, "[[database]]"); err != nil {
		t.Fatal(err)
	}
	links, _ = s.DB.OutgoingLinks(from.ID)
	if len(links) != 0 {
		t.Errorf("expected link removed, got %+v", links)
	}
}

func TestGraph(t *testing.T) {
	s := newSvc(t)
	w, _ := s.CreateWiki("demo", "Demo")
	_, _ = s.CreatePage(w, "B", "B.md", "# B\n")
	_, _ = s.CreatePage(w, "A", "A.md", "# A\n[[B]]\n")
	nodes, edges, err := s.DB.Graph(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 2 {
		t.Errorf("nodes = %d, want 2", len(nodes))
	}
	if len(edges) != 1 {
		t.Errorf("edges = %d, want 1 (%+v)", len(edges), edges)
	}
}

func TestUnsafePathRejected(t *testing.T) {
	s := newSvc(t)
	w, _ := s.CreateWiki("demo", "Demo")
	if _, err := s.CreatePage(w, "Bad", "../escape.md", "x"); !errors.Is(err, ErrUnsafePath) {
		t.Errorf("expected ErrUnsafePath, got %v", err)
	}
}
