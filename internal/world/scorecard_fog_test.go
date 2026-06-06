package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFogScoreResult(t *testing.T) {
	dir := t.TempDir()

	// Missing report → clean 100 (predates fog-checking), never 0.
	if r := FogScoreResult(filepath.Join(dir, "nope.md")); r.Score != 100 {
		t.Errorf("missing report score = %d, want 100", r.Score)
	}

	// CLEAN verdict → 100.
	clean := filepath.Join(dir, "clean.md")
	if err := os.WriteFile(clean,
		[]byte("# Fog-of-war report — installment\n\n**Verdict:** CLEAN — no sealed material stated.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := FogScoreResult(clean); r.Score != 100 {
		t.Errorf("clean score = %d, want 100", r.Score)
	}

	// Leaks weigh heavily: 100 - 30*2 - 5*1 = 35.
	leaky := filepath.Join(dir, "leaky.md")
	if err := os.WriteFile(leaky,
		[]byte("# Fog-of-war report\n\n**Verdict:** 3 finding(s) — 2 leak, 1 watch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := FogScoreResult(leaky)
	if r.Score != 35 {
		t.Errorf("leaky score = %d, want 35", r.Score)
	}
	if len(r.Violations) == 0 || r.Violations[0].Severity != "breaking" {
		t.Errorf("expected a breaking violation, got %+v", r.Violations)
	}
}
