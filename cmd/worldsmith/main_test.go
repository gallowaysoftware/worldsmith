package main

import (
	"strings"
	"testing"
)

func TestValidateNarrator(t *testing.T) {
	if err := validateNarrator("am_fenrir"); err != nil {
		t.Errorf("known voice rejected: %v", err)
	}
	err := validateNarrator("am_fenfir") // common typo
	if err == nil {
		t.Fatal("typo'd narrator voice should be rejected early")
	}
	if !strings.Contains(err.Error(), "am_fenfir") {
		t.Errorf("error should name the bad voice: %v", err)
	}
}

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
