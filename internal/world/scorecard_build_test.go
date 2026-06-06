package world

import (
	"os"
	"testing"
)

func TestBuildScorecard_AxesMatchedByName(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	if err := os.MkdirAll(l.InstallmentDir(1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.InstallmentFile(1, "story.md"),
		[]byte("The slate-grey water rose against the harbour wall. She watched it climb."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.InstallmentFile(1, "continuity_report.md"),
		[]byte(`{"findings":[{"severity":"BREAKING","conflict":"contradicts the bible"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.InstallmentFile(1, "fog_report.md"),
		[]byte(`{"findings":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	card := BuildScorecard(l, 1)

	prose := ResultByAxis(card, AxisProse)
	if AxisName(prose) != AxisProse {
		t.Errorf("prose axis not found by name; summary=%q", prose.Summary)
	}
	cont := ResultByAxis(card, AxisContinuity)
	if cont.Score != 70 { // one breaking
		t.Errorf("continuity score = %d, want 70", cont.Score)
	}
	fog := ResultByAxis(card, AxisFog)
	if fog.Score != 100 {
		t.Errorf("fog score = %d, want 100 (empty findings)", fog.Score)
	}
	// A non-existent axis returns the zero value, not a false match.
	if got := ResultByAxis(card, "nope"); AxisName(got) != "" {
		t.Errorf("unexpected match for missing axis: %q", got.Summary)
	}
	// Overall is the minimum axis (continuity's 70 here).
	if ov := Overall(card); ov != 70 {
		t.Errorf("Overall = %d, want 70", ov)
	}
}

func TestBuildScorecard_OmitsFogWhenNoReport(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	if err := os.MkdirAll(l.InstallmentDir(1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.InstallmentFile(1, "story.md"), []byte("words here for prose."), 0o644); err != nil {
		t.Fatal(err)
	}
	// No fog_report.md → the fog axis must be absent.
	card := BuildScorecard(l, 1)
	if AxisName(ResultByAxis(card, AxisFog)) == AxisFog {
		t.Errorf("fog axis present despite no fog_report.md: %+v", card.Results)
	}
}
