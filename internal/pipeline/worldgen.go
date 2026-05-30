package pipeline

import (
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// WorldGenConfig drives `worldsmith worldgen`: auto-author a world bible +
// cast from a creative theme. The breadth phase of the content mill — the
// human-authored `init` path stays available; this is the LLM-authored one.
type WorldGenConfig struct {
	Theme string
}

// BuildWorldGen emits a single-stage pipeline that turns a theme into a
// structured world JSON (name, setting, tone, visual_style, factions, rules,
// characters with voice_ids, locations). The CLI renders that JSON into the
// standard worldsmith world.md + characters.json so every existing command
// works on the result. LLM-only (no GPU contention with image gen).
func BuildWorldGen(cfg WorldGenConfig) (*vamp.Pipeline, error) {
	p := vamp.New("worldsmith-worldgen").
		Describe("Auto-author a world bible + cast from a theme (content-mill breadth).")

	p.Input("theme", vamp.Required(), vamp.WithDefault(cfg.Theme),
		vamp.Describe("Creative theme / niche the world is built around."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during generation")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("seed_world").
		Capability("long_form").
		PromptFS(PromptsFS, "seed_world.md").
		OutputFormatJSON().
		Output("world.json").
		// Warm temperature for variety across runs (the mill wants many
		// distinct worlds), but the JSON gate + retry keep it parseable.
		Param("temperature", 0.9).
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
