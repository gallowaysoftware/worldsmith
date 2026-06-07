package pipeline

import (
	"fmt"
	"os"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// RevealPlanConfig drives the reveal-pacing planner — the LLM pass that figures out, for
// one book, the chapter where each sealed secret should first reach the page. It reads the
// author's private notebook (the secrets), the book's overall reveal-license (what may be
// revealed across this book at all), and every chapter's beat (what each scene
// dramatizes), and emits a per-chapter reveal-license. The model is good at this PLANNING
// even though it leaks during creative writing — same reason the continuity checker can
// diagnose contradictions a blind draft won't avoid. Output is a reviewable JSON plan the
// CLI folds into arc.json's chapters[].reveals.
type RevealPlanConfig struct {
	WorldFile    string
	NotebookFile string
	BookN        int
	BookTitle    string
	// BookReveals is the rendered book-wide reveal-license (bullets). Anything sealed and
	// NOT covered by it belongs to a later book and must never be licensed here.
	BookReveals string
	// ChaptersRendered is the rendered chapter list (n, title, hook, beats, constraints)
	// so the planner can pace by what each chapter actually dramatizes.
	ChaptersRendered string
	OutputName       string // default "reveal_plan.json"
}

// BuildRevealPlan constructs the one-stage planner pipeline. Output JSON:
// {"chapters":[{"n":1,"reveals":["<license sentence>"]},{"n":2,"reveals":[]}]}.
func BuildRevealPlan(cfg RevealPlanConfig) (*vamp.Pipeline, error) {
	if cfg.OutputName == "" {
		cfg.OutputName = "reveal_plan.json"
	}
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	p := vamp.New("worldsmith-reveal-plan").
		Describe("Plan per-chapter reveal pacing for a book from the notebook + chapter beats.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the assembled author's notebook (the sealed secrets to pace)."))
	p.Input("book_n", vamp.WithDefault(fmt.Sprintf("%d", cfg.BookN)),
		vamp.Describe("This book's 1-based number."))
	p.Input("book_title", vamp.WithDefault(cfg.BookTitle),
		vamp.Describe("This book's title."))
	p.Input("book_reveals", vamp.WithDefault(fallbackStr(cfg.BookReveals, "(no book-wide reveal license — keep everything sealed)")),
		vamp.Describe("The book-wide reveal license: sealed material this book may surface at all."))
	p.Input("chapters", vamp.WithDefault(cfg.ChaptersRendered),
		vamp.Describe("The book's chapters: what each one dramatizes (n/title/hook/beats/constraints)."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during reveal planning")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("reveal_plan").
		Capability("long_form").
		PromptFS(PromptsFS, "reveal_plan.md").
		OutputFormatJSON().
		Output(cfg.OutputName).
		Param("temperature", 0.3).
		// One bounded license per chapter across a ~25-chapter book is sizable JSON.
		Param("max_tokens", 12288).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	return p.Build()
}
