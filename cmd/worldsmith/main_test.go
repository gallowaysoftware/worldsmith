package main

import (
	"strings"
	"testing"

	"github.com/gallowaysoftware/worldsmith/internal/world"
)

func TestSanitizeSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The Sealed Gate", "the-sealed-gate"},
		{"  Already-Clean  ", "already-clean"},
		{"under_scores_too", "under-scores-too"},
		{"Punctuation! & symbols?", "punctuation-symbols"},
		{"multiple   spaces", "multiple-spaces"},
		{"---leading-trailing---", "leading-trailing"},
		{"CaseFold123", "casefold123"},
		{"", ""},
		{"!!!", ""},
	}
	for _, c := range cases {
		if got := sanitizeSlug(c.in); got != c.want {
			t.Errorf("sanitizeSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNextEventID(t *testing.T) {
	if got := nextEventID(nil); got != "evt_0001" {
		t.Errorf("nextEventID(nil) = %q, want evt_0001", got)
	}
	events := []world.Event{
		{ID: "evt_0001"},
		{ID: "evt_0007"},
		{ID: "not-an-evt-id"}, // ignored
		{ID: "evt_0003"},
	}
	if got := nextEventID(events); got != "evt_0008" {
		t.Errorf("nextEventID = %q, want evt_0008 (max+1)", got)
	}
}

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
