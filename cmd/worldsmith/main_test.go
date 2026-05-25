package main

import "testing"

func TestSanitizeFilenameFragment(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The First Hour", "The First Hour"},
		{"Slashes/in/title", "Slashesintitle"},
		{"Backslashes\\too", "Backslashestoo"},
		{`Quotes"and:colons`, "Quotesandcolons"},
		{"Pipe|and?question", "Pipeandquestion"},
		{"  Leading and trailing  ", "Leading and trailing"},
		{"Multiple    spaces", "Multiple spaces"},
		{".dotty.", "dotty"},
		{"", ""},
		{"   ", ""},
		{"Title\twith\ttabs", "Title with tabs"},
	}
	for _, c := range cases {
		if got := sanitizeFilenameFragment(c.in); got != c.want {
			t.Errorf("sanitizeFilenameFragment(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
