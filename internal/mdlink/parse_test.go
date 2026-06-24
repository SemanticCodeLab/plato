package mdlink

import "testing"

func TestParse(t *testing.T) {
	content := `# Title

See [[Database]] and [[database|the DB]] and [[Auth Page]].
Relative link [Auth](./auth.md) and [Deep](../shared/notes.md).
External [Site](https://example.com) should be ignored.
Image ![pic](./x.png) should be ignored.
Anchor [local](#section) ignored. PDF [doc](./a.pdf) ignored.
`
	refs := Parse(content)

	var wiki, rel int
	for _, r := range refs {
		switch r.Kind {
		case KindWiki:
			wiki++
		case KindRelative:
			rel++
		}
	}
	if wiki != 3 {
		t.Errorf("wiki links = %d, want 3", wiki)
	}
	if rel != 2 {
		t.Errorf("relative links = %d, want 2 (got refs: %+v)", rel, refs)
	}

	// Check label parsing.
	var found bool
	for _, r := range refs {
		if r.Kind == KindWiki && r.Target == "database" && r.Label == "the DB" {
			found = true
		}
	}
	if !found {
		t.Errorf("did not parse [[database|the DB]] correctly: %+v", refs)
	}
}

func TestParseIgnoresCode(t *testing.T) {
	content := "Real link [[Database]].\n\n" +
		"```toml\n[[edges]]\nfrom = \"a\"\n[link](./x.md)\n```\n\n" +
		"Inline `[[NotALink]]` and `[code](./y.md)` ignored.\n" +
		"After fence [[Guide]].\n"
	refs := Parse(content)

	got := map[string]bool{}
	for _, r := range refs {
		got[r.Target] = true
	}
	if !got["Database"] || !got["Guide"] {
		t.Errorf("expected Database and Guide links, got %+v", refs)
	}
	for _, bad := range []string{"edges", "NotALink", "./x.md", "./y.md"} {
		if got[bad] {
			t.Errorf("link %q inside code should have been ignored: %+v", bad, refs)
		}
	}
	if len(refs) != 2 {
		t.Errorf("expected exactly 2 links, got %d: %+v", len(refs), refs)
	}
}

func TestIsRelativeMD(t *testing.T) {
	accept := []string{"./auth.md", "../x/y.md", "docs/page.md", "page.md#section"}
	reject := []string{"https://x.com/a.md", "//host/a.md", "mailto:a@b.md", "page.txt", "#anchor", ""}
	for _, s := range accept {
		if !isRelativeMD(s) {
			t.Errorf("isRelativeMD(%q) = false, want true", s)
		}
	}
	for _, s := range reject {
		if isRelativeMD(s) {
			t.Errorf("isRelativeMD(%q) = true, want false", s)
		}
	}
}
