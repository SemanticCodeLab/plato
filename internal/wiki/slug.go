package wiki

import (
	"fmt"
	"regexp"
	"strings"
)

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// Slugify converts a title to a slug: lowercase, punctuation/spaces -> "-",
// collapse repeats, trim "-"; empty -> "page".
func Slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "page"
	}
	return s
}

// existsFunc reports whether a candidate slug is already taken.
type existsFunc func(slug string) (bool, error)

// UniqueSlug returns Slugify(title), appending -2, -3, ... until exists reports
// the slug is free.
func UniqueSlug(title string, exists existsFunc) (string, error) {
	base := Slugify(title)
	candidate := base
	for i := 2; ; i++ {
		taken, err := exists(candidate)
		if err != nil {
			return "", err
		}
		if !taken {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
}
