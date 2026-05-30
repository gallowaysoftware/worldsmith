package pipeline

import (
	"fmt"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// ArcConfig drives the arc generator — the novel-scope counterpart to
// the brief generator. One LLM pass turns the world bible plus a scope
// premise into arc.json (title + premise + ordered chapter beats),
// which `worldsmith novel` then runs chapter by chapter. The output is
// a draft for the human to edit before it runs.
type ArcConfig struct {
	// Premise is the author's scope for the whole book. Empty = let the
	// model propose the strongest novel-length arc the world supports.
	Premise        string
	TargetChapters int
	WorldFile      string
	CharactersFile string
	CanonFile      string
}

// BuildArc constructs the one-stage pipeline that drafts arc.json.
func BuildArc(cfg ArcConfig) (*vamp.Pipeline, error) {
	if cfg.TargetChapters == 0 {
		cfg.TargetChapters = 12
	}

	p := vamp.New("worldsmith-arc").
		Describe("Propose a novel's chapter arc (arc.json) from the world bible + a scope premise.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md (may be empty)."))
	p.Input("premise", vamp.WithDefault(cfg.Premise),
		vamp.Describe("Optional author scope/premise for the whole novel."))
	p.Input("target_chapters", vamp.WithDefault(fmt.Sprintf("%d", cfg.TargetChapters)),
		vamp.Describe("Approximate chapter count for the arc."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during arc generation")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("propose_arc").
		Capability("long_form").
		PromptFS(PromptsFS, "propose_arc.md").
		OutputFormatJSON().
		Output("arc.json").
		Param("temperature", 0.6).
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
