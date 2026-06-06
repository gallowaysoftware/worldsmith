package world

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Series is the top-level plan for a multi-book work: a narrative arc (the
// key events and the final state the whole series drives toward) plus an
// ordered list of books. Where `arc.json` holds the chapter beats (the
// `series plan` step generates those from each book here), `series.json` is
// the human-authored spine: what each book is about, how long, whose eyes it
// is told through, and which sealed material it is licensed to reveal.
//
// A "book" is a contiguous range of the flat global chapter numbering — there
// is no separate per-book chapter tree on disk. Canon, priors, and resume all
// stay flat (installments/NNN), so the per-chapter pipeline is reused
// unchanged; a book is just a chapter range with its own reveal-license and
// POV roster.
type Series struct {
	SeriesTitle string       `json:"series_title"`
	Arc         SeriesArc    `json:"arc"`
	Books       []SeriesBook `json:"books"`
}

// SeriesArc is the whole-series spine: the key events it must hit and the
// state it ends in. The book outlines are planned to honour it.
type SeriesArc struct {
	KeyEvents  []string `json:"key_events,omitempty"`
	FinalState string   `json:"final_state,omitempty"`
}

// SeriesBook is one book's direction. TargetChapters × TargetWordsPerChapter
// sets the length (≈ audio hours); POVRoster is the ensemble the planner may
// narrate chapters through (the fog lever — a POV who does not know a sealed
// cluster cannot leak it); Reveals is the per-book reveal-license applied to
// EVERY chapter in the book (the cluster mechanism at series scale).
type SeriesBook struct {
	N                     int      `json:"n"`
	Title                 string   `json:"title"`
	Premise               string   `json:"premise,omitempty"`
	ArcSummary            string   `json:"arc_summary,omitempty"`
	TargetChapters        int      `json:"target_chapters"`
	TargetWordsPerChapter int      `json:"target_words_per_chapter,omitempty"`
	POVRoster             []string `json:"pov_roster,omitempty"`
	Reveals               []string `json:"reveals,omitempty"`
}

// SeriesFile is the path to series.json under the world root.
func (l Layout) SeriesFile() string { return filepath.Join(l.Root, "series.json") }

// LoadSeries reads + parses series.json. Returns (zero, false, nil) when the
// file is absent so callers distinguish "no series" from a parse error.
func LoadSeries(l Layout) (Series, bool, error) {
	raw, err := os.ReadFile(l.SeriesFile())
	if err != nil {
		if os.IsNotExist(err) {
			return Series{}, false, nil
		}
		return Series{}, false, err
	}
	var s Series
	if err := json.Unmarshal(raw, &s); err != nil {
		return Series{}, true, fmt.Errorf("parse %s: %w", l.SeriesFile(), err)
	}
	return s, true, nil
}

// ScaffoldSeries writes a stub series.json when none exists. Idempotent.
func ScaffoldSeries(l Layout) error {
	if _, err := os.Stat(l.SeriesFile()); err == nil {
		return nil
	}
	return os.WriteFile(l.SeriesFile(), []byte(seriesStub()), 0o644)
}

// Book returns the SeriesBook with the given 1-based number, or false.
func (s Series) Book(n int) (SeriesBook, bool) {
	for _, b := range s.Books {
		if b.N == n {
			return b, true
		}
	}
	return SeriesBook{}, false
}

func seriesStub() string {
	return `{
  "series_title": "<series title>",
  "arc": {
    "key_events": [
      "<a major event the series must hit>",
      "<another>"
    ],
    "final_state": "<where the world stands when the series ends>"
  },
  "books": [
    {
      "n": 1,
      "title": "<book 1 title (optional; the planner can name it)>",
      "premise": "<one or two sentences: what book 1 is about>",
      "arc_summary": "<the ordered beats book 1 must cover, prose or bullets>",
      "target_chapters": 25,
      "target_words_per_chapter": 5500,
      "pov_roster": ["<character slug>", "<character slug>"],
      "reveals": ["<sealed material LICENSED for every chapter in this book; empty = reveal nothing sealed>"]
    }
  ]
}
`
}
