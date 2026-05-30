package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadArc_Missing(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	_, ok, err := LoadArc(l)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false for missing arc.json")
	}
}

func TestLoadArc_Parses(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	doc := `{"title":"The Drowned Coast","premise":"a map that lies","chapters":[{"title":"The Tide","hook":"h","beats":["a","b"],"pov":"Asha"}]}`
	if err := os.WriteFile(l.ArcFile(), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	a, ok, err := LoadArc(l)
	if err != nil || !ok {
		t.Fatalf("LoadArc ok=%v err=%v", ok, err)
	}
	if a.Title != "The Drowned Coast" {
		t.Errorf("title = %q", a.Title)
	}
	if len(a.Chapters) != 1 || a.Chapters[0].Title != "The Tide" {
		t.Errorf("chapters = %+v", a.Chapters)
	}
	if len(a.Chapters[0].Beats) != 2 {
		t.Errorf("beats = %+v", a.Chapters[0].Beats)
	}
}

func TestLoadArc_BadJSON(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	if err := os.WriteFile(l.ArcFile(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, ok, err := LoadArc(l)
	if err == nil {
		t.Errorf("expected parse error")
	}
	if !ok {
		t.Errorf("ok should be true (file existed) even on parse error")
	}
}

func TestScaffoldArc(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	if err := ScaffoldArc(l); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(l.ArcFile()); err != nil {
		t.Fatalf("arc.json not written: %v", err)
	}
	// Idempotent: don't clobber existing content.
	custom := []byte(`{"title":"mine"}`)
	if err := os.WriteFile(l.ArcFile(), custom, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ScaffoldArc(l); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(l.ArcFile())
	if string(got) != string(custom) {
		t.Errorf("ScaffoldArc clobbered existing arc.json")
	}
}

func TestRenderBriefFromBeat(t *testing.T) {
	b := ArcBeat{
		Title:       "The Harbour Closes",
		Hook:        "The blockade ends in fire.",
		Beats:       []string{"ships burn", "Asha flees"},
		POV:         "Asha, third-limited",
		Constraints: []string{"no magic reveal"},
	}
	out := RenderBriefFromBeat(1, b)
	if !strings.HasPrefix(out, "# 001 — The Harbour Closes") {
		t.Errorf("H1 wrong:\n%s", out)
	}
	for _, want := range []string{"The blockade ends in fire.", "- ships burn", "- Asha flees", "Asha, third-limited", "no magic reveal"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered brief missing %q:\n%s", want, out)
		}
	}
	// BriefTitle should recover the title from the rendered H1.
	tmp := filepath.Join(t.TempDir(), "001.md")
	if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := BriefTitle(tmp); got != "The Harbour Closes" {
		t.Errorf("BriefTitle = %q, want The Harbour Closes", got)
	}
}

func TestRenderBriefFromBeat_Defaults(t *testing.T) {
	out := RenderBriefFromBeat(3, ArcBeat{})
	if !strings.Contains(out, "# 003 — Chapter 3") {
		t.Errorf("expected default chapter title:\n%s", out)
	}
}
