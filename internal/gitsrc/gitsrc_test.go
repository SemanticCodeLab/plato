package gitsrc

import "testing"

func TestValidateURL(t *testing.T) {
	ok := []string{
		"https://github.com/example/repo.git",
		"https://gitlab.com/group/repo.git",
		"https://codeberg.org/user/repo",
	}
	for _, u := range ok {
		if err := ValidateURL(u); err != nil {
			t.Errorf("ValidateURL(%q) = %v, want nil", u, err)
		}
	}

	bad := []string{
		"https://user:pass@github.com/org/repo.git",
		"file:///tmp/repo",
		"git@github.com:org/repo.git",
		"ssh://git@github.com/org/repo.git",
		"/tmp/local/path",
		"http://github.com/org/repo.git", // not https
		"",
	}
	for _, u := range bad {
		if err := ValidateURL(u); err == nil {
			t.Errorf("ValidateURL(%q) = nil, want rejection", u)
		}
	}
}
