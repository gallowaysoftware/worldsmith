package pipeline

import (
	"fmt"
	"os"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// OutlineConfig drives a single outline-only run. The CLI uses it for
// candidate rerank: generate several outlines (varying temperature),
// score them, and feed the winner to the full story pipeline via
// StoryConfig.OutlineJSON.
type OutlineConfig struct {
	WorldFile             string
	CharactersFile        string
	CanonFile             string
	CanonRelevantFile     string
	PriorsFile            string
	BriefFile             string
	HistoricalContextFile string
	NotebookFile          string
	// Temperature lets the caller diversify candidates — sampling the
	// same prompt at different temperatures yields genuinely different
	// plans to choose between rather than near-duplicates.
	Temperature float64
	// TargetWords sets the installment's target prose length (drives the
	// per-scene budgets the outline emits). 0 = default (~10,000).
	TargetWords int
}

// BuildOutline constructs a one-stage pipeline that produces just the
// scene plan (outline.json) for an installment. Mirrors the full
// pipeline's outline_story stage so a candidate is representative of
// what the real run would generate.
func BuildOutline(cfg OutlineConfig) (*vamp.Pipeline, error) {
	if cfg.CanonRelevantFile == "" {
		cfg.CanonRelevantFile = cfg.CanonFile
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.4
	}
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	if cfg.TargetWords == 0 {
		cfg.TargetWords = 10000
	}

	p := vamp.New("worldsmith-outline").
		Describe("Generate one scene-plan candidate for an installment.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md."))
	p.Input("canon_relevant_file", vamp.WithDefault(cfg.CanonRelevantFile),
		vamp.Describe("Path to the relevance-filtered canon view (defaults to full canon)."))
	p.Input("priors_file", vamp.Required(), vamp.WithDefault(cfg.PriorsFile),
		vamp.Describe("Path to concatenated prior-installment summaries."))
	p.Input("brief_file", vamp.Required(), vamp.WithDefault(cfg.BriefFile),
		vamp.Describe("Path to this installment's brief.md."))
	p.Input("historical_context_file", vamp.WithDefault(cfg.HistoricalContextFile),
		vamp.Describe("Path to the pre-filtered timeline view."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the assembled author's notebook (private dossiers). Empty/DevNull when none."))
	p.Input("target_words", vamp.WithDefault(fmt.Sprintf("%d", cfg.TargetWords)),
		vamp.Describe("Target prose length for this installment; the per-scene budgets must sum to it."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during outline")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("outline_story").
		Capability("long_form").
		PromptFS(PromptsFS, "outline_story.md").
		OutputFormatJSON().
		Output("outline.json").
		Param("temperature", cfg.Temperature).
		Param("max_tokens", 8192).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	return p.Build()
}
