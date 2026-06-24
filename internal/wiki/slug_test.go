package wiki

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Authentication":  "authentication",
		"User Model":      "user-model",
		"API Gateway!":    "api-gateway",
		"  Hello, World ": "hello-world",
		"":                "page",
		"!!!":             "page",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueSlug(t *testing.T) {
	taken := map[string]bool{"user-model": true, "user-model-2": true}
	exists := func(s string) (bool, error) { return taken[s], nil }
	got, err := UniqueSlug("User Model", exists)
	if err != nil {
		t.Fatal(err)
	}
	if got != "user-model-3" {
		t.Errorf("got %q, want user-model-3", got)
	}
}
