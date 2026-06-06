package world

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Arc is the optional fixed beat-sheet for a novel-scope work. Where a
// `story` is driven one installment at a time by a hand-written
// brief.md, a `novel` is driven top-down by arc.json: a title, a
// premise, and an ordered list of chapter beats. `worldsmith novel`
// turns each beat into a per-chapter brief and runs the same
// installment pipeline over it, so canon, summaries, continuity, and
// metrics roll forward chapter to chapter exactly as they do across
// installments.
//
// arc.json is only consulted by the novel flow; `story` ignores it
// (per-installment briefs drive there). It's the Dramatron-style
// hierarchy — premise → chapter beats → prose — that gives a long
// work its spine.
type Arc struct {
	Title   string `json:"title"`
	Premise string `json:"premise,omitempty"`
	// Chapters is the flat chapter list for a single-novel work (the `novel`
	// command). Books is the per-book grouping for a multi-book series (the
	// `series` command); when Books is set, the global chapter sequence is the
	// books' Chapters concatenated in order and Chapters is left empty.
	Chapters []ArcBeat `json:"chapters,omitempty"`
	Books    []ArcBook `json:"books,omitempty"`
}

// ArcBook groups a contiguous run of chapters into one book of a series. It is
// the `series plan` output. Reveals (the per-book reveal-license) and
// TargetWords (per-chapter length) are copied from series.json so the write
// step reads a self-contained arc.json — given a global chapter number it can
// find the owning book's license + length without re-reading series.json.
type ArcBook struct {
	N           int       `json:"n"`
	Title       string    `json:"title,omitempty"`
	Premise     string    `json:"premise,omitempty"`
	Reveals     []string  `json:"reveals,omitempty"`
	TargetWords int       `json:"target_words,omitempty"`
	Chapters    []ArcBeat `json:"chapters"`
}

// FlatChapters returns every chapter in global order: the books' chapters
// concatenated (series mode) or the flat Chapters (single-novel mode).
func (a Arc) FlatChapters() []ArcBeat {
	if len(a.Books) == 0 {
		return a.Chapters
	}
	var out []ArcBeat
	for _, b := range a.Books {
		out = append(out, b.Chapters...)
	}
	return out
}

// BookForChapter maps a 1-based GLOBAL chapter number to its owning ArcBook
// (series mode). Returns false in single-novel mode or when n is out of range.
func (a Arc) BookForChapter(n int) (ArcBook, bool) {
	if len(a.Books) == 0 || n < 1 {
		return ArcBook{}, false
	}
	end := 0
	for _, b := range a.Books {
		end += len(b.Chapters)
		if n <= end {
			return b, true
		}
	}
	return ArcBook{}, false
}

// ArcBeat is one chapter's direction — the same shape as a brief, in
// structured form. RenderBriefFromBeat turns it into the markdown a
// brief.md would contain so the rest of the pipeline can't tell the
// difference between an arc-driven chapter and a hand-written one.
type ArcBeat struct {
	Title       string   `json:"title"`
	Hook        string   `json:"hook,omitempty"`
	Beats       []string `json:"beats,omitempty"`
	POV         string   `json:"pov,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
}

// LoadArc reads + parses arc.json. Returns (zero, nil) when the file
// doesn't exist so callers can distinguish "no arc" (story scope, or a
// novel that needs scaffolding) from a parse error.
func LoadArc(l Layout) (Arc, bool, error) {
	raw, err := os.ReadFile(l.ArcFile())
	if err != nil {
		if os.IsNotExist(err) {
			return Arc{}, false, nil
		}
		return Arc{}, false, err
	}
	var a Arc
	if err := json.Unmarshal(raw, &a); err != nil {
		return Arc{}, true, fmt.Errorf("parse %s: %w", l.ArcFile(), err)
	}
	return a, true, nil
}

// ScaffoldArc writes a stub arc.json when none exists, so a user
// starting a novel has the shape to fill in. Idempotent — never
// clobbers an existing file.
func ScaffoldArc(l Layout) error {
	if _, err := os.Stat(l.ArcFile()); err == nil {
		return nil
	}
	return os.WriteFile(l.ArcFile(), []byte(arcStub()), 0o644)
}

// RenderBriefFromBeat produces the brief.md content for chapter n from
// an arc beat. The output mirrors the hand-written brief format
// (briefStub) so BriefTitle, the writer prompt, and --publish-to all
// behave identically. The H1 carries the chapter number + title so the
// published filename reads "001 - <title>.m4b".
func RenderBriefFromBeat(n int, b ArcBeat) string {
	var sb strings.Builder
	title := strings.TrimSpace(b.Title)
	if title == "" {
		title = fmt.Sprintf("Chapter %d", n)
	}
	fmt.Fprintf(&sb, "# %03d — %s\n\n", n, title)

	fmt.Fprintf(&sb, "## Hook\n\n%s\n\n", fallback(b.Hook, "(no hook specified)"))

	sb.WriteString("## What happens\n\n")
	if len(b.Beats) == 0 {
		sb.WriteString("  - (no beats specified)\n\n")
	} else {
		for _, beat := range b.Beats {
			fmt.Fprintf(&sb, "  - %s\n", strings.TrimSpace(beat))
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "## Pov / lens\n\n%s\n\n", fallback(b.POV, "(author's choice)"))

	sb.WriteString("## Constraints\n\n")
	if len(b.Constraints) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for _, c := range b.Constraints {
			fmt.Fprintf(&sb, "  - %s\n", strings.TrimSpace(c))
		}
	}
	return sb.String()
}

func fallback(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return strings.TrimSpace(s)
}

// RenderSeriesChapterBrief renders chapter n's brief.md for the series flow:
// YAML frontmatter carrying the book's reveal-license and target length, then
// the beat body (identical to RenderBriefFromBeat). ParseBrief reads the
// frontmatter so the per-chapter pipeline picks up the reveals + word target
// without any other change. reveals strings are JSON-encoded (valid YAML
// scalars) so colons/quotes in a reveal can't break the frontmatter.
func RenderSeriesChapterBrief(n int, b ArcBeat, reveals []string, targetWords int) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	if len(reveals) > 0 {
		sb.WriteString("reveals:\n")
		for _, r := range reveals {
			q, _ := json.Marshal(strings.TrimSpace(r))
			fmt.Fprintf(&sb, "  - %s\n", q)
		}
	}
	if targetWords > 0 {
		fmt.Fprintf(&sb, "target_words: %d\n", targetWords)
	}
	sb.WriteString("---\n\n")
	sb.WriteString(RenderBriefFromBeat(n, b))
	return sb.String()
}

func arcStub() string {
	return `{
  "title": "<novel title>",
  "premise": "<one or two sentences: the spine of the whole book>",
  "chapters": [
    {
      "title": "<chapter title>",
      "hook": "<one sentence — the reason to keep listening>",
      "beats": [
        "<what happens, beat 1>",
        "<beat 2>",
        "<beat 3>"
      ],
      "pov": "<whose head are we in this chapter>",
      "constraints": [
        "<anything the LLM should NOT do this chapter>"
      ]
    }
  ]
}
`
}
