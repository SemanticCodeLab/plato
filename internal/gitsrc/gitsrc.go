// Package gitsrc provides a deliberately small Git import mechanism for public
// HTTPS Markdown repositories. It is not a credential manager: SSH, file://,
// and credential-bearing URLs are rejected.
package gitsrc

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ErrInvalidURL is returned for any disallowed repository URL.
var ErrInvalidURL = errors.New("invalid git url")

// ValidateURL enforces the MVP constraint: only public https URLs, no embedded
// credentials, no ssh/file/scp-style remotes.
func ValidateURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("%w: empty", ErrInvalidURL)
	}
	// scp-style "git@host:org/repo.git" has no scheme.
	if strings.HasPrefix(raw, "git@") || strings.HasPrefix(raw, "ssh://") {
		return fmt.Errorf("%w: ssh not supported", ErrInvalidURL)
	}
	if strings.HasPrefix(raw, "file://") {
		return fmt.Errorf("%w: file urls not supported", ErrInvalidURL)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("%w: only https is supported", ErrInvalidURL)
	}
	if u.User != nil {
		return fmt.Errorf("%w: credentials in url not allowed", ErrInvalidURL)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: missing host", ErrInvalidURL)
	}
	return nil
}

// reposDir returns the managed clone directory for a project, as an absolute path
// so git commands work regardless of the process working directory.
func reposDir(wikiDir, slug string) string {
	p := filepath.Join(wikiDir, ".repos", slug)
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// Clone validates and shallow-clones url@branch into <wikiDir>/.repos/<slug>,
// replacing any existing clone. Returns the Markdown source root (clone dir
// joined with subdir, if any).
func Clone(wikiDir, slug, rawURL, branch, subdir string) (string, error) {
	if err := ValidateURL(rawURL); err != nil {
		return "", err
	}
	dest := reposDir(wikiDir, slug)
	if err := os.RemoveAll(dest); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	args := []string{"clone", "--depth", "1"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, rawURL, dest)
	if err := run(wikiDir, args...); err != nil {
		return "", err
	}
	return sourceRoot(dest, subdir)
}

// Pull updates an existing clone and returns the Markdown source root. The branch
// is reset hard to the remote to avoid merge handling (MVP: no conflict UI).
func Pull(wikiDir, slug, branch, subdir string) (string, error) {
	dest := reposDir(wikiDir, slug)
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		return "", fmt.Errorf("no clone for project %q", slug)
	}
	b := branch
	if b == "" {
		b = "HEAD"
	}
	if err := run(dest, "fetch", "--depth", "1", "origin", b); err != nil {
		return "", err
	}
	if err := run(dest, "reset", "--hard", "FETCH_HEAD"); err != nil {
		return "", err
	}
	return sourceRoot(dest, subdir)
}

// sourceRoot resolves the subdir under a clone, guarding against traversal.
func sourceRoot(clone, subdir string) (string, error) {
	if subdir == "" {
		return clone, nil
	}
	clean := filepath.Clean("/" + subdir) // force-rooted, strips ".."
	root := filepath.Join(clone, clean)
	if !strings.HasPrefix(root, clone) {
		return "", fmt.Errorf("%w: bad subdir", ErrInvalidURL)
	}
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("subdir %q not found in repo", subdir)
	}
	return root, nil
}

// run executes git in dir with a timeout, surfacing stderr on failure.
func run(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0", // never prompt for credentials
		"GIT_ASKPASS=true",
	)
	out, err := combinedWithTimeout(cmd, 120*time.Second)
	if err != nil {
		return fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(out))
	}
	return nil
}

func combinedWithTimeout(cmd *exec.Cmd, d time.Duration) (string, error) {
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
		return string(out), err
	case <-time.After(d):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("git timed out after %s", d)
	}
}
