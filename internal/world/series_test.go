package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSeries_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	l := Layout{Root: dir}
	js := `{
  "series_title": "Test Series",
  "arc": {"key_events": ["e1","e2"], "final_state": "resolved"},
  "books": [
    {"n":1,"title":"B1","target_chapters":3,"target_words_per_chapter":5500,
     "pov_roster":["ila","kavin"],"reveals":["the breeding cluster"]},
    {"n":2,"title":"B2","target_chapters":2,"reveals":["the rip"]}
  ]
}`
	if err := os.WriteFile(l.SeriesFile(), []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	s, ok, err := LoadSeries(l)
	if err != nil || !ok {
		t.Fatalf("LoadSeries ok=%v err=%v", ok, err)
	}
	if s.SeriesTitle != "Test Series" || len(s.Books) != 2 {
		t.Fatalf("bad parse: %+v", s)
	}
	if s.Arc.FinalState != "resolved" || len(s.Arc.KeyEvents) != 2 {
		t.Errorf("arc not parsed: %+v", s.Arc)
	}
	b1, ok := s.Book(1)
	if !ok || b1.TargetChapters != 3 || b1.TargetWordsPerChapter != 5500 ||
		len(b1.POVRoster) != 2 || len(b1.Reveals) != 1 {
		t.Errorf("book 1 fields: %+v", b1)
	}
	if _, ok := s.Book(9); ok {
		t.Errorf("Book(9) should be absent")
	}
}

func TestLoadSeries_Absent(t *testing.T) {
	_, ok, err := LoadSeries(Layout{Root: t.TempDir()})
	if ok || err != nil {
		t.Fatalf("absent series should be ok=false err=nil; got ok=%v err=%v", ok, err)
	}
}

func TestArc_FlatChapters_And_BookForChapter(t *testing.T) {
	beat := func(t string) ArcBeat { return ArcBeat{Title: t} }
	// Series-mode arc: book 1 has 3 chapters, book 2 has 2 → global 1..5.
	a := Arc{
		Books: []ArcBook{
			{N: 1, Title: "B1", Reveals: []string{"breeding"}, TargetWords: 5500,
				Chapters: []ArcBeat{beat("c1"), beat("c2"), beat("c3")}},
			{N: 2, Title: "B2", Reveals: []string{"rip"}, TargetWords: 6000,
				Chapters: []ArcBeat{beat("c4"), beat("c5")}},
		},
	}
	flat := a.FlatChapters()
	if len(flat) != 5 || flat[0].Title != "c1" || flat[4].Title != "c5" {
		t.Fatalf("FlatChapters wrong: %d %+v", len(flat), flat)
	}
	cases := []struct {
		n          int
		wantBook   int
		wantReveal string
	}{
		{1, 1, "breeding"}, {3, 1, "breeding"}, {4, 2, "rip"}, {5, 2, "rip"},
	}
	for _, c := range cases {
		b, ok := a.BookForChapter(c.n)
		if !ok || b.N != c.wantBook || b.Reveals[0] != c.wantReveal {
			t.Errorf("BookForChapter(%d) = %+v ok=%v, want book %d reveal %q", c.n, b, ok, c.wantBook, c.wantReveal)
		}
	}
	if _, ok := a.BookForChapter(6); ok {
		t.Errorf("BookForChapter(6) out of range should be false")
	}
	if _, ok := a.BookForChapter(0); ok {
		t.Errorf("BookForChapter(0) should be false")
	}

	// Single-novel mode (flat Chapters, no Books): FlatChapters = Chapters,
	// BookForChapter returns false (no series grouping).
	flatArc := Arc{Chapters: []ArcBeat{beat("x"), beat("y")}}
	if len(flatArc.FlatChapters()) != 2 {
		t.Errorf("flat arc FlatChapters wrong")
	}
	if _, ok := flatArc.BookForChapter(1); ok {
		t.Errorf("flat arc BookForChapter should be false (no books)")
	}
}

func TestScaffoldSeries_Idempotent(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	if err := ScaffoldSeries(l); err != nil {
		t.Fatal(err)
	}
	// Write a sentinel, scaffold again, confirm not clobbered.
	if err := os.WriteFile(l.SeriesFile(), []byte(`{"series_title":"keep"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ScaffoldSeries(l); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(l.SeriesFile())
	if string(b) != `{"series_title":"keep"}` {
		t.Errorf("ScaffoldSeries clobbered existing file: %s", b)
	}
	_ = filepath.Base(l.SeriesFile())
}
