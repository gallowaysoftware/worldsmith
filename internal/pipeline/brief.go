package pipeline

import (
	"fmt"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// BriefConfig drives the brief generator — a single LLM pass that
// proposes the next installment's brief.md from where the story stands
// (world bible + canon + prior summaries), optionally nudged by a
// one-line steer. The output is a draft for the human to edit; nothing
// runs it automatically.
type BriefConfig struct {
	InstallmentNumber int
	TargetWords       int
	// Steer is the author's optional one-line direction ("this one is
	// about the extraction beginning"). Empty = let the model free-run
	// the arc from canon + priors.
	Steer string

	WorldFile             string
	CharactersFile        string
	CanonFile             string
	PriorsFile            string
	HistoricalContextFile string
	// ExemplarBriefFile is the most recent existing brief, shown to the
	// model as a house-style + continuity exemplar. os.DevNull when
	// none exists.
	ExemplarBriefFile string
}

// BuildBrief constructs the one-stage pipeline that drafts a brief.
func BuildBrief(cfg BriefConfig) (*vamp.Pipeline, error) {
	if cfg.TargetWords == 0 {
		cfg.TargetWords = 6500
	}

	p := vamp.New("worldsmith-brief").
		Describe("Propose the next installment's brief from where the story stands.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md."))
	p.Input("priors_file", vamp.Required(), vamp.WithDefault(cfg.PriorsFile),
		vamp.Describe("Path to concatenated prior-installment summaries."))
	p.Input("historical_context_file", vamp.WithDefault(cfg.HistoricalContextFile),
		vamp.Describe("Path to the timeline view through the current year."))
	p.Input("exemplar_brief_file", vamp.WithDefault(cfg.ExemplarBriefFile),
		vamp.Describe("Path to the most recent brief (format/continuity exemplar); os.DevNull when none."))
	p.Input("steer", vamp.WithDefault(cfg.Steer),
		vamp.Describe("Optional one-line author direction for this installment."))
	p.Input("installment_number", vamp.WithDefault(fmt.Sprintf("%d", cfg.InstallmentNumber)),
		vamp.Describe("1-indexed installment number this brief is for."))
	p.Input("target_words", vamp.WithDefault(fmt.Sprintf("%d", cfg.TargetWords)),
		vamp.Describe("Target prose length the beats' budgets should sum to."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during brief generation")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("propose_brief").
		Capability("long_form").
		PromptFS(PromptsFS, "propose_brief.md").
		Output("brief.md").
		// Warmer than the prose stages: this is generative planning, and
		// we want range across re-runs. enable_thinking off so no CoT
		// preamble lands in the brief markdown.
		Param("temperature", 0.7).
		Param("max_tokens", 6144).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    2,
			InitialBackoff: 5 * time.Second,
			RetryOn:        []string{"transient"},
		})

	return p.Build()
}
