package pipeline

import (
	"fmt"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// BenchStoryConfig drives a "prose-only" run of the story pipeline.
// Same write_story + edit_story stages as the full BuildStory; skips
// TTS / cover / mix / canon_delta / showrunner / summarize so the
// model A/B finishes in ~5-10 min instead of ~30. The trade-off is
// you don't get an audiobook out — but you also don't pay a Kokoro
// pass on every bench iteration when the variable being studied is
// prose quality.
type BenchStoryConfig struct {
	WorldFile             string
	CharactersFile        string
	CanonFile             string
	CanonRelevantFile     string
	PriorsFile            string
	BriefFile             string
	HistoricalContextFile string
}

// BuildBenchStory constructs a stripped story pipeline for prose
// A/B testing. Outputs: draft.md (write_story) + story.md (edit_story).
// No JSON-format gates, no cover art, no TTS. Single profile
// activation (`long_form`).
//
// Use case: `worldsmith bench` runs this with capabilities.yaml
// temporarily mapped to each candidate, capturing the story.md
// output side-by-side for human-judged comparison.
func BuildBenchStory(cfg BenchStoryConfig) (*vamp.Pipeline, error) {
	if cfg.CanonRelevantFile == "" {
		cfg.CanonRelevantFile = cfg.CanonFile
	}
	p := vamp.New("worldsmith-bench-story").
		Describe("Prose-only run of the story pipeline (write + edit) for candidate-model A/B testing.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md."))
	p.Input("canon_relevant_file", vamp.WithDefault(cfg.CanonRelevantFile),
		vamp.Describe("Path to the relevance-filtered canon view (defaults to full canon)."))
	p.Input("priors_file", vamp.Required(), vamp.WithDefault(cfg.PriorsFile),
		vamp.Describe("Path to priors.md."))
	p.Input("brief_file", vamp.Required(), vamp.WithDefault(cfg.BriefFile),
		vamp.Describe("Path to this installment's brief.md."))
	p.Input("historical_context_file", vamp.WithDefault(cfg.HistoricalContextFile),
		vamp.Describe("Path to the pre-filtered timeline view."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during write_story + edit_story")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	thinkingOff := map[string]any{"enable_thinking": false}

	// Bench mirrors the production story pipeline's prose stages
	// (outline_story → write_story → edit_story) so the bench is
	// fair to the production prompt set. The bench skips the
	// audiobook stages (showrunner / TTS / mix / cover) since prose
	// quality is the variable under study.
	outline := p.Text("outline_story").
		Capability("long_form").
		PromptFS(PromptsFS, "outline_story.md").
		OutputFormatJSON().
		Output("outline.json").
		Param("temperature", 0.4).
		Param("max_tokens", 8192).
		Param("chat_template_kwargs", thinkingOff).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	draft := p.Text("write_story").
		Capability("long_form").
		After(outline).
		PromptFS(PromptsFS, "write_story.md").
		Output("draft.md").
		Param("temperature", 0.8).
		Param("max_tokens", 24576).
		Param("chat_template_kwargs", thinkingOff).
		Param("repetition_penalty", 1.1).
		Param("presence_penalty", 0.3).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    2,
			InitialBackoff: 5 * time.Second,
			RetryOn:        []string{"transient"},
		})

	p.Text("edit_story").
		Capability("long_form").
		After(draft).
		PromptFS(PromptsFS, "edit_story.md").
		Output("story.md").
		Param("temperature", 0.4).
		Param("max_tokens", 24576).
		Param("chat_template_kwargs", thinkingOff).
		Param("repetition_penalty", 1.1).
		Param("presence_penalty", 0.3).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    2,
			InitialBackoff: 5 * time.Second,
			RetryOn:        []string{"transient"},
		})

	return p.Build()
}

// ErrBenchOutputMissing is returned by the bench CLI when a candidate's
// run dir doesn't contain story.md after the pipeline reportedly
// succeeded.
var ErrBenchOutputMissing = fmt.Errorf("bench candidate produced no story.md")
