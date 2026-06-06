package pipeline

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// BookOutlineConfig drives the per-book chapter planner — the series-scope
// counterpart to BuildArc. One LLM pass turns a book's premise + arc-summary
// (plus the world bible, canon, sealed notebook, the whole-series arc, and a
// recap of earlier books) into that book's ordered chapter beats. The planner
// assigns each chapter a POV from the roster (the fog lever — a POV who does
// not know a sealed cluster cannot leak it) and honours the book's
// reveal-license. Output is a reviewable draft the human edits before writing.
type BookOutlineConfig struct {
	SeriesTitle       string
	SeriesArc         string // rendered: key events + final state
	BookN             int
	BookTitle         string
	Premise           string
	ArcSummary        string
	TargetChapters    int
	POVRoster         []string // character slugs the planner may narrate through
	Reveals           []string // the book's reveal-license (applies to every chapter)
	PriorBooksSummary string   // what earlier books cover (empty for book 1)
	WorldFile         string
	CharactersFile    string
	CanonFile         string
	NotebookFile      string
}

// BuildBookOutline constructs the one-stage pipeline that drafts one book's
// chapter beats as JSON ({"chapters":[{title,hook,beats,pov,constraints}]}).
func BuildBookOutline(cfg BookOutlineConfig) (*vamp.Pipeline, error) {
	if cfg.TargetChapters == 0 {
		cfg.TargetChapters = 25
	}
	if cfg.CanonFile == "" {
		cfg.CanonFile = os.DevNull
	}
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}

	p := vamp.New("worldsmith-book-outline").
		Describe("Plan one book's chapter beats (POV- and reveal-aware) from its premise + the series arc.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md (revealed-to-reader ledger; may be empty)."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the sealed author's notebook."))
	p.Input("series_title", vamp.WithDefault(cfg.SeriesTitle),
		vamp.Describe("The series title."))
	p.Input("series_arc", vamp.WithDefault(cfg.SeriesArc),
		vamp.Describe("The whole-series arc: key events + final state."))
	p.Input("book_n", vamp.WithDefault(fmt.Sprintf("%d", cfg.BookN)),
		vamp.Describe("This book's 1-based number."))
	p.Input("book_title", vamp.WithDefault(cfg.BookTitle),
		vamp.Describe("This book's title (may be blank — the planner can name it)."))
	p.Input("premise", vamp.WithDefault(cfg.Premise),
		vamp.Describe("This book's premise."))
	p.Input("arc_summary", vamp.WithDefault(cfg.ArcSummary),
		vamp.Describe("The ordered beats this book must cover."))
	p.Input("target_chapters", vamp.WithDefault(fmt.Sprintf("%d", cfg.TargetChapters)),
		vamp.Describe("Approximate chapter count for this book."))
	p.Input("pov_roster", vamp.WithDefault(renderList(cfg.POVRoster)),
		vamp.Describe("Character slugs the planner may assign as chapter POV."))
	p.Input("reveals", vamp.WithDefault(renderList(cfg.Reveals)),
		vamp.Describe("The book's reveal-license: sealed material it may put on the page."))
	p.Input("prior_books", vamp.WithDefault(fallbackStr(cfg.PriorBooksSummary, "(this is the first book)")),
		vamp.Describe("Recap of what earlier books cover."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during book outlining")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("book_outline").
		Capability("long_form").
		PromptFS(PromptsFS, "book_outline.md").
		OutputFormatJSON().
		Output(fmt.Sprintf("book_%02d_outline.json", cfg.BookN)).
		Param("temperature", 0.6).
		// ~25 chapter beats with hooks/beats/pov/constraints is sizable JSON.
		Param("max_tokens", 16384).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	return p.Build()
}

// renderList turns a slice into a markdown bullet list for a prompt input,
// or a placeholder when empty.
func renderList(items []string) string {
	if len(items) == 0 {
		return "(none specified)"
	}
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(it))
	}
	return b.String()
}

func fallbackStr(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
