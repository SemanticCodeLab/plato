// Package sync imports a directory of Markdown files into a Plato wiki.
package sync

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SourceFile is a discovered Markdown file relative to the sync root.
type SourceFile struct {
	RelPath string // slash-separated, relative to the source dir
	AbsPath string
}

// Walk returns all *.md files under dir, with rel paths relative to dir.
func Walk(dir string) ([]SourceFile, error) {
	var out []SourceFile
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, SourceFile{RelPath: filepath.ToSlash(rel), AbsPath: p})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReadSource reads a source file's content.
func ReadSource(abs string) (string, error) {
	b, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
