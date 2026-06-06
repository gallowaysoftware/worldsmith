package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeInstallmentFile(t *testing.T, l Layout, n int, name, body string) {
	t.Helper()
	if err := os.MkdirAll(l.InstallmentDir(n), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.InstallmentFile(n, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAssemblePriors(t *testing.T) {
	l := Layout{Root: t.TempDir()}

	// n=1 → no priors.
	if got, err := AssemblePriors(l, 1); err != nil || got != "" {
		t.Fatalf("AssemblePriors(1) = %q, %v; want empty", got, err)
	}

	writeInstallmentFile(t, l, 1, "summary.md", "Recap of one.")
	// Gap at 2 (no summary) must be skipped, not error.
	writeInstallmentFile(t, l, 3, "summary.md", "Recap of three.")

	got, err := AssemblePriors(l, 4)
	if err != nil {
		t.Fatalf("AssemblePriors: %v", err)
	}
	if !strings.Contains(got, "Installment 1") || !strings.Contains(got, "Recap of one.") {
		t.Errorf("missing installment 1: %q", got)
	}
	if !strings.Contains(got, "Installment 3") || !strings.Contains(got, "Recap of three.") {
		t.Errorf("missing installment 3: %q", got)
	}
	if strings.Contains(got, "Installment 2") {
		t.Errorf("gap installment 2 should be absent: %q", got)
	}
	if strings.Contains(got, "Installment 4") {
		t.Errorf("AssemblePriors must stop before the target installment: %q", got)
	}
}

func TestNextInstallment(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	if n, err := NextInstallment(l); err != nil || n != 1 {
		t.Fatalf("empty world NextInstallment = %d, %v; want 1", n, err)
	}
	writeInstallmentFile(t, l, 1, "episode.m4b", "x")
	writeInstallmentFile(t, l, 2, "episode.m4b", "x")
	// installment 3 dir exists but is not finished (no m4b) → it's next.
	if err := os.MkdirAll(l.InstallmentDir(3), 0o755); err != nil {
		t.Fatal(err)
	}
	if n, err := NextInstallment(l); err != nil || n != 3 {
		t.Fatalf("NextInstallment = %d, %v; want 3", n, err)
	}
}

func TestNextScene(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	if n, err := NextScene(l); err != nil || n != 1 {
		t.Fatalf("empty world NextScene = %d, %v; want 1", n, err)
	}
	if err := os.MkdirAll(l.SceneDir(1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.SceneFile(1, "final.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := NextScene(l); err != nil || n != 2 {
		t.Fatalf("NextScene = %d, %v; want 2", n, err)
	}
}

func TestMergeGeneratedTimeline(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("eras.json", `{"eras":[{"slug":"founding","name":"The Founding","start":0,"end":100}]}`)
	write("anchors.json", `{"events":[{"id":"evt_0001","year":10,"summary":"founding","confidence":"canon"}]}`)
	write("regional.json", `{"events":[{"id":"evt_0002","year":20,"summary":"a regional thing"}]}`)
	write("personal.json", `{"events":[]}`)
	write("visibilities.json", `{"visibilities":[{"id":"evt_0002","tier":"regional","rumoured_as":"a rumour"}]}`)

	eras, events, err := MergeGeneratedTimeline(dir)
	if err != nil {
		t.Fatalf("MergeGeneratedTimeline: %v", err)
	}
	if len(eras) != 1 || eras[0].Slug != "founding" {
		t.Errorf("eras = %+v", eras)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	for _, e := range events {
		// Every machine-proposed event is forced to proposed + source llm regardless of
		// what the model claimed, so a model-supplied confidence:canon can't bypass review.
		if e.Confidence != ConfidenceProposed {
			t.Errorf("event %s confidence = %q, want proposed", e.ID, e.Confidence)
		}
		if e.Source != "llm" {
			t.Errorf("event %s source = %q, want llm", e.ID, e.Source)
		}
	}
	// Visibility applied where present; defaulted to common otherwise.
	byID := map[string]Event{}
	for _, e := range events {
		byID[e.ID] = e
	}
	if byID["evt_0002"].Visibility.Tier != TierRegional {
		t.Errorf("evt_0002 tier = %q, want regional", byID["evt_0002"].Visibility.Tier)
	}
	if byID["evt_0001"].Visibility.Tier != TierCommon {
		t.Errorf("evt_0001 (no visibility) tier = %q, want common default", byID["evt_0001"].Visibility.Tier)
	}
}

func TestArcChapterRevealRoundTrip(t *testing.T) {
	// RenderSeriesChapterBrief writes reveals + target_words into YAML frontmatter that
	// ParseBrief reads back — the round-trip the per-chapter pipeline depends on.
	beat := ArcBeat{Title: "The Gate", Hook: "Something stirs.", Beats: []string{"a", "b"}}
	reveals := []string{`The "sealed" name: Kesh`, "A second reveal"}
	content := RenderSeriesChapterBrief(2, beat, reveals, 3500)

	dir := t.TempDir()
	p := filepath.Join(dir, "002.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	front, body, err := ParseBrief(p)
	if err != nil {
		t.Fatalf("ParseBrief: %v", err)
	}
	if len(front.Reveals) != 2 || front.Reveals[0] != `The "sealed" name: Kesh` {
		t.Errorf("reveals round-trip wrong: %#v", front.Reveals)
	}
	if !strings.Contains(body, "The Gate") {
		t.Errorf("brief body missing the beat title: %q", body)
	}
}
