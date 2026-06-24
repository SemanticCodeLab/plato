package wiki

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

// HashContent returns the "sha256:" prefixed hex digest of content.
func HashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ReadFile reads a Markdown file and returns its content and content hash.
func ReadFile(abs string) ([]byte, string, error) {
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, "", err
	}
	return b, HashContent(b), nil
}

// WriteFileAtomic writes content to abs atomically: temp file in the same dir,
// then rename over the target. Returns the new content hash.
func WriteFileAtomic(abs string, content []byte) (string, error) {
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".plato-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		return "", err
	}
	return HashContent(content), nil
}
