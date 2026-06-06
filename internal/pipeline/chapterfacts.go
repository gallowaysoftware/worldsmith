package pipeline

import (
	"os"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// ChapterFactsConfig drives the per-chapter grounding pass. Before a chapter is
// written, one low-temperature LLM pass reads the chapter's brief (its beats)
// against the world bible, canon, sealed notebook, and timeline, and extracts a
// tight, imperative fact-sheet: the EXACT canon and mechanics this chapter's
// events touch — the things the writer must get right and must NOT improvise
// (how entanglement behaves, who is captured when, which ships die here, the
// timeline placement, a non-human species' distinct tech). The fact-sheet is
// pinned into the per-scene writer so dense-canon chapters stop tripping on
// improvised mechanics (the continuity failure mode on the contact-event chapter).
type ChapterFactsConfig struct {
	BriefFile             string
	WorldFile             string
	CanonFile             string
	NotebookFile          string
	PriorsFile            string
	HistoricalContextFile string
}

// BuildChapterFacts constructs the one-stage pipeline that emits chapter_facts.md.
func BuildChapterFacts(cfg ChapterFactsConfig) (*vamp.Pipeline, error) {
	if cfg.CanonFile == "" {
		cfg.CanonFile = os.DevNull
	}
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	if cfg.PriorsFile == "" {
		cfg.PriorsFile = os.DevNull
	}
	if cfg.HistoricalContextFile == "" {
		cfg.HistoricalContextFile = os.DevNull
	}

	p := vamp.New("worldsmith-chapter-facts").
		Describe("Extract the exact canon + mechanics this chapter's events touch, so the writer doesn't improvise them.")

	p.Input("brief_file", vamp.Required(), vamp.WithDefault(cfg.BriefFile),
		vamp.Describe("Path to this chapter's brief.md (its beats)."))
	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("canon_file", vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md (revealed-to-reader ledger)."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the sealed author's notebook."))
	p.Input("priors_file", vamp.WithDefault(cfg.PriorsFile),
		vamp.Describe("Path to prior-chapter summaries."))
	p.Input("historical_context_file", vamp.WithDefault(cfg.HistoricalContextFile),
		vamp.Describe("Path to the pre-filtered timeline view."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during fact extraction")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("chapter_facts").
		Capability("long_form").
		PromptFS(PromptsFS, "chapter_facts.md").
		Output("chapter_facts.md").
		// Low temperature: this is factual extraction, not creative.
		Param("temperature", 0.1).
		Param("max_tokens", 4096).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	return p.Build()
}
