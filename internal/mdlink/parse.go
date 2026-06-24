// Package mdlink parses and resolves Markdown cross-links: [[wikilinks]] and
// relative .md links. External URLs and images are ignored.
package mdlink

import (
	"regexp"
	"strings"
)

// Kind values mirror store kinds.
const (
	KindWiki     = "wiki"
	KindRelative = "relative"
)

// Ref is a parsed link reference, before resolution.
type Ref struct {
	Kind   string // wiki | relative
	Target string // wikilink target (title or slug) OR relative .md path
	Label  string // optional display label
	Raw    string // original source text
}

var (
	// [[Target]] or [[Target|Label]]
	wikiRe = regexp.MustCompile(`\[\[([^\]\|]+?)(?:\|([^\]]+?))?\]\]`)
	// [Label](dest) — image variant ![...](...) excluded via the leading-! check below.
	mdRe = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^)\s]+)\)`)
)

// Parse extracts all wiki and relative .md links from Markdown content. Links
// inside fenced code blocks and inline code spans are ignored, so syntax like
// TOML's [[table]] arrays does not produce spurious wikilinks.
func Parse(content string) []Ref {
	var refs []Ref

	content = stripCode(content)

	for _, m := range wikiRe.FindAllStringSubmatch(content, -1) {
		target := strings.TrimSpace(m[1])
		if target == "" {
			continue
		}
		refs = append(refs, Ref{
			Kind:   KindWiki,
			Target: target,
			Label:  strings.TrimSpace(m[2]),
			Raw:    m[0],
		})
	}

	for _, m := range mdRe.FindAllStringSubmatch(content, -1) {
		isImage := m[1] == "!"
		label := strings.TrimSpace(m[2])
		dest := strings.TrimSpace(m[3])
		if isImage {
			continue // images ignored
		}
		if !isRelativeMD(dest) {
			continue // external URLs / anchors / non-.md ignored
		}
		refs = append(refs, Ref{
			Kind:   KindRelative,
			Target: dest,
			Label:  label,
			Raw:    m[0],
		})
	}

	return refs
}

// stripCode blanks out fenced code blocks (``` or ~~~) and inline code spans,
// preserving line structure so links in surrounding prose still parse. Blanked
// regions keep their length replaced by spaces/newlines so nothing inside them
// is matched as a link.
func stripCode(content string) string {
	lines := strings.Split(content, "\n")
	var fence string // current fence marker ("```" or "~~~"), empty when outside
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if fence == "" {
			if strings.HasPrefix(trimmed, "```") {
				fence = "```"
				lines[i] = ""
				continue
			}
			if strings.HasPrefix(trimmed, "~~~") {
				fence = "~~~"
				lines[i] = ""
				continue
			}
			lines[i] = stripInlineCode(line)
		} else {
			// Inside a fence: blank everything; a closing fence ends it.
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// stripInlineCode replaces `...` spans on a single line with spaces.
func stripInlineCode(line string) string {
	var b strings.Builder
	inCode := false
	for _, r := range line {
		if r == '`' {
			inCode = !inCode
			b.WriteByte(' ')
			continue
		}
		if inCode {
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isRelativeMD reports whether dest is a relative link to a .md file.
func isRelativeMD(dest string) bool {
	if dest == "" {
		return false
	}
	// Drop any anchor fragment for the extension check.
	if i := strings.IndexByte(dest, '#'); i >= 0 {
		dest = dest[:i]
	}
	if dest == "" {
		return false
	}
	lower := strings.ToLower(dest)
	if !strings.HasSuffix(lower, ".md") {
		return false
	}
	// Reject schemes (http:, https:, mailto:, etc) and protocol-relative URLs.
	if strings.Contains(dest, "://") || strings.HasPrefix(dest, "//") {
		return false
	}
	if i := strings.IndexByte(dest, ':'); i >= 0 {
		// A ':' before any '/' indicates a scheme.
		if s := strings.IndexByte(dest, '/'); s < 0 || i < s {
			return false
		}
	}
	return true
}
