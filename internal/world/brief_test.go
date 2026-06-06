package world

import (
	"os"
	"strings"
	"testing"
)

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	body := []byte("# Just a brief\n\nThis installment opens at the docks.\n")
	front, gotBody, err := splitFrontmatter(body)
	if err != nil {
		t.Fatal(err)
	}
	if front.YearOverride != 0 || front.POVRegion != "" || len(front.OnStageActors) != 0 {
		t.Errorf("brief without frontmatter should yield zero BriefFront; got %+v", front)
	}
	if !strings.Contains(gotBody, "Just a brief") {
		t.Errorf("body lost: %q", gotBody)
	}
}

func TestSplitFrontmatter_HappyPath(t *testing.T) {
	body := []byte("---\nyear_override: 412\npov_region: veld\non_stage_actors:\n  - asha\n  - veska\n---\n# Body\n\nProse goes here.\n")
	front, gotBody, err := splitFrontmatter(body)
	if err != nil {
		t.Fatalf("splitFrontmatter: %v", err)
	}
	if front.YearOverride != 412 {
		t.Errorf("YearOverride = %d, want 412", front.YearOverride)
	}
	if front.POVRegion != "veld" {
		t.Errorf("POVRegion = %q, want veld", front.POVRegion)
	}
	if len(front.OnStageActors) != 2 || front.OnStageActors[0] != "asha" {
		t.Errorf("OnStageActors = %v", front.OnStageActors)
	}
	if !strings.Contains(gotBody, "Prose goes here") {
		t.Errorf("body lost: %q", gotBody)
	}
	if strings.Contains(gotBody, "year_override") {
		t.Errorf("frontmatter leaked into body: %q", gotBody)
	}
}

func TestSplitFrontmatter_UnclosedFrontmatterErrors(t *testing.T) {
	body := []byte("---\nyear_override: 412\n\nbody without closing delim\n")
	_, _, err := splitFrontmatter(body)
	if err == nil {
		t.Fatal("expected error on unclosed frontmatter")
	}
	if !strings.Contains(err.Error(), "without a closing") {
		t.Errorf("error should mention missing closing delim; got: %v", err)
	}
}

func TestSplitFrontmatter_BadYAMLErrors(t *testing.T) {
	body := []byte("---\nthis: is: : not: valid: yaml:\n---\n# Body\n")
	_, _, err := splitFrontmatter(body)
	if err == nil {
		t.Fatal("expected YAML parse error")
	}
}

func TestFilterOptsFromBrief_PrefersYearOverride(t *testing.T) {
	cal := Calendar{CurrentYear: 100}
	brief := BriefFront{YearOverride: 50, POVRegion: "veld", OnStageActors: []string{"asha"}}
	opts := FilterOptsFromBrief(brief, cal)
	if opts.YearCutoff != 50 {
		t.Errorf("YearCutoff = %d, want 50 (override should win)", opts.YearCutoff)
	}
	if opts.POVRegion != "veld" {
		t.Errorf("POVRegion = %q", opts.POVRegion)
	}
	if len(opts.OnStageActors) != 1 {
		t.Errorf("OnStageActors = %v", opts.OnStageActors)
	}
}

func TestFilterOptsFromBrief_FallsBackToCurrentYear(t *testing.T) {
	cal := Calendar{CurrentYear: 100}
	brief := BriefFront{POVRegion: "veld"}
	opts := FilterOptsFromBrief(brief, cal)
	if opts.YearCutoff != 100 {
		t.Errorf("YearCutoff = %d, want 100 (current_year fallback)", opts.YearCutoff)
	}
}

func TestWriteHistoricalContext(t *testing.T) {
	tmp := t.TempDir()
	events := []Event{
		{ID: "a", Year: 100, Kind: "founding", Summary: "First lighting",
			Visibility: Visibility{Tier: TierCommon}, Confidence: ConfidenceCanon},
		{ID: "b", Year: 200, Kind: "war", Region: "veld", Summary: "Harbour blockade",
			Visibility: Visibility{Tier: TierRegional, RumouredAs: "siege ended by miracle"},
			Confidence: ConfidenceCanon},
		{ID: "c", Year: 500, Kind: "future", Summary: "Should be filtered out",
			Visibility: Visibility{Tier: TierCommon}, Confidence: ConfidenceCanon},
	}
	opts := FilterOpts{YearCutoff: 300, HasCutoff: true, POVRegion: "marsh"}
	path, err := WriteHistoricalContext(tmp, events, opts)
	if err != nil {
		t.Fatalf("WriteHistoricalContext: %v", err)
	}
	if !strings.HasSuffix(path, "historical_context.md") {
		t.Errorf("path = %q", path)
	}
	// Re-read to confirm the rendering shape.
	raw, err := readFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "Year cutoff: 300") {
		t.Errorf("missing year cutoff header: %s", raw)
	}
	if !strings.Contains(raw, "100 | founding | First lighting") {
		t.Errorf("missing common-knowledge event: %s", raw)
	}
	if !strings.Contains(raw, "200 | war | RUMOUR: siege ended by miracle") {
		t.Errorf("missing rumour line: %s", raw)
	}
	if strings.Contains(raw, "Should be filtered out") {
		t.Errorf("future event leaked past year cutoff: %s", raw)
	}
}

func TestStripInstallmentPrefix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"001 — The First Hour", "The First Hour"},
		{"001 - The First Hour", "The First Hour"},
		{"001. The First Hour", "The First Hour"},
		{"42 – Foo Bar", "Foo Bar"},                        // en-dash + hyphen variants
		{"The First Hour", "The First Hour"},               // no prefix → unchanged
		{"001The First Hour", "001The First Hour"},         // digits not followed by separator → unchanged
		{"001 missing separator", "001 missing separator"}, // digits + space but no — / - / . → unchanged
	}
	for _, c := range cases {
		if got := stripInstallmentPrefix(c.in); got != c.want {
			t.Errorf("stripInstallmentPrefix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBriefTitle(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"with-frontmatter", "---\nyear_override: 3600\n---\n\n# 001 — The First Hour\n\nBody.", "The First Hour"},
		{"no-frontmatter", "# Foo\n\nBody.", "Foo"},
		{"no-h1", "Body without heading.", ""},
		{"empty", "", ""},
		{"only-h2", "## Subheading\n\nBody.", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := dir + "/brief_" + c.name + ".md"
			if err := os.WriteFile(path, []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := BriefTitle(path); got != c.want {
				t.Errorf("BriefTitle(%q) = %q, want %q", c.name, got, c.want)
			}
		})
	}

	// Non-existent file: returns empty string, not error.
	if got := BriefTitle(dir + "/does-not-exist.md"); got != "" {
		t.Errorf("BriefTitle(missing) = %q, want empty", got)
	}
}

// readFile is a test helper — kept here so the production code keeps
// its minimal surface.
func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
