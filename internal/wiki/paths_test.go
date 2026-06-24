package wiki

import "testing"

func TestCleanRelPathReject(t *testing.T) {
	bad := []string{
		"../x.md",
		"../../etc/passwd.md",
		"/etc/passwd.md",
		"a.md/../../b.md",
		"notes/../../x.md",
		"no-extension",
		"image.png",
		"",
	}
	for _, p := range bad {
		if _, err := CleanRelPath(p); err == nil {
			t.Errorf("CleanRelPath(%q) = nil err, want rejection", p)
		}
	}
}

func TestCleanRelPathAccept(t *testing.T) {
	good := map[string]string{
		"Home.md":                   "Home.md",
		"docs/API.md":               "docs/API.md",
		"systems/authentication.md": "systems/authentication.md",
		"./Home.md":                 "Home.md",
	}
	for in, want := range good {
		got, err := CleanRelPath(in)
		if err != nil {
			t.Errorf("CleanRelPath(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("CleanRelPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAbsPathContainment(t *testing.T) {
	if _, err := AbsPath("/srv/data/demo", "Home.md"); err != nil {
		t.Errorf("valid path rejected: %v", err)
	}
}
