package world

import (
	"strings"
	"testing"
)

func TestOpenRejectsTraversalSlug(t *testing.T) {
	// Point DefaultRoot at a scratch dir so a valid slug doesn't litter
	// the real state dir.
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	bad := []string{
		"../../etc",
		"..",
		"a/b",
		`a\b`,
		"foo/../bar",
		"",
		"Mixed-Case",
		"has space",
	}
	for _, slug := range bad {
		if _, err := Open(slug); err == nil {
			t.Errorf("Open(%q) = nil error, want rejection", slug)
		}
	}

	if _, err := Open("good-slug-1"); err != nil {
		t.Errorf("Open(%q) = %v, want success", "good-slug-1", err)
	}
}

func TestValidSlug(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"good-slug-1", true},
		{"a", true},
		{"", false},
		{"../escape", false},
		{strings.ToUpper("X"), false},
		{"under_score", false},
	} {
		if got := ValidSlug(c.in); got != c.want {
			t.Errorf("ValidSlug(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
