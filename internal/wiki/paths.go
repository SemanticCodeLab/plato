package wiki

import (
	"errors"
	"path/filepath"
	"strings"
)

// ErrUnsafePath is returned when a page rel_path fails validation.
var ErrUnsafePath = errors.New("unsafe path")

// CleanRelPath validates and normalizes a page path. It must be relative, end in
// .md, contain no "..", and stay inside the wiki root. Returns the slash-cleaned
// relative path on success.
func CleanRelPath(rel string) (string, error) {
	if rel == "" {
		return "", ErrUnsafePath
	}
	// Normalize separators to forward slashes for consistent storage.
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, "/") {
		return "", ErrUnsafePath // absolute
	}
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return "", ErrUnsafePath
	}
	// Reject any traversal component before normalization collapses it.
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", ErrUnsafePath
		}
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", ErrUnsafePath
	}
	return clean, nil
}

// AbsPath joins a validated relative path under the wiki root, guaranteeing the
// result stays within root.
func AbsPath(root, relClean string) (string, error) {
	abs := filepath.Join(root, filepath.FromSlash(relClean))
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absAbs, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	if absAbs != rootAbs && !strings.HasPrefix(absAbs, rootAbs+string(filepath.Separator)) {
		return "", ErrUnsafePath
	}
	return abs, nil
}
