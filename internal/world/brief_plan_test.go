package world

import (
	"os"
	"testing"
)

func touchBrief(t *testing.T, l Layout, n int) {
	t.Helper()
	if err := os.MkdirAll(l.BriefsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.BriefFile(n), []byte("# brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNextBriefNumber_Empty(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	n, err := NextBriefNumber(l)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("next on empty = %d, want 1", n)
	}
}

func TestNextBriefNumber_Sequential(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	touchBrief(t, l, 1)
	touchBrief(t, l, 2)
	n, err := NextBriefNumber(l)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("next after 1,2 = %d, want 3", n)
	}
}

func TestNextBriefNumber_FillsGap(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	touchBrief(t, l, 1)
	touchBrief(t, l, 3)
	n, err := NextBriefNumber(l)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("next with gap at 2 = %d, want 2", n)
	}
}

func TestLatestBriefNumber(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	if got := LatestBriefNumber(l); got != 0 {
		t.Errorf("latest on empty = %d, want 0", got)
	}
	touchBrief(t, l, 1)
	touchBrief(t, l, 2)
	if got := LatestBriefNumber(l); got != 2 {
		t.Errorf("latest = %d, want 2", got)
	}
}
