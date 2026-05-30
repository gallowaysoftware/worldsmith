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
	Title    string    `json:"title"`
	Premise  string    `json:"premise,omitempty"`
	Chapters []ArcBeat `json:"chapters"`
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
