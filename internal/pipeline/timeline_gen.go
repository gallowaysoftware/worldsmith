package pipeline

import (
	"fmt"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// TimelineGenConfig drives one run of the LLM timeline generator.
// The CLI builds this from the world layout + the chosen output dir.
type TimelineGenConfig struct {
	// WorldFile is the path to world.md (immutable bible).
	WorldFile string
	// CharactersFile is the path to characters.json. Used by the
	// personalise pass to weave named cast into history.
	CharactersFile string
	// ExistingTimelineFile is the optional path to an existing
	// timeline.json the generator should respect (don't contradict
	// already-canon eras / events). Empty string means "no prior
	// timeline" — the loader's behaviour for a missing file at the
	// world root.
	ExistingTimelineFile string
}

// BuildTimelineGen constructs the five-pass timeline generation
// pipeline. Each pass writes a JSON file to the run dir; the CLI
// reads them all after the pipeline completes and merges into
// timeline.json as proposed events.
//
// Stages:
//
//	seed_eras          (text, json) → eras.json
//	seed_anchors       (text, json) → anchors.json
//	elaborate_regional (text, json) → regional.json
//	personalise        (text, json) → personal.json
//	fog_pass           (text, json) → visibilities.json
//
// All five stages use the `long_form` capability — the same Qwen3.6
// long-context model the story pipeline uses, since these are
// long-context creative tasks. Temperature steps down at each pass:
// the first three are inventive (eras / anchors / consequences),
// the fourth interpolates between cast + history (slightly hotter),
// and the fifth is classification (cool).
//
// Token budgets are conservative — a verbose Qwen3 CoT preamble plus
// the actual JSON output usually fits within max_tokens, but if the
// fog pass sees 50+ events the output may push 16K. Bump
// max_tokens on `fog_pass` if you see truncated JSON.
func BuildTimelineGen(cfg TimelineGenConfig) (*vamp.Pipeline, error) {
	p := vamp.New("worldsmith-timeline-gen").
		Describe("Generate a historical timeline for a worldsmith world: eras → anchors → regional consequences → personal anchors → fog-of-war pass.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md (immutable bible)."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("existing_timeline_file", vamp.WithDefault(cfg.ExistingTimelineFile),
		vamp.Describe("Path to existing timeline.json (optional; generator extends it without contradicting)."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~28GB during all five passes")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	// Default retry policy used on every pass — the JSON gate
	// breaks if the model emits a CoT preamble or stray prose; an
	// invalid_output retry usually rescues it.
	jsonRetry := &vamp.RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: 5 * time.Second,
		MaxBackoff:     30 * time.Second,
		RetryOn:        []string{"transient", "invalid_output"},
	}

	// chat_template_kwargs.enable_thinking=false: on EXL3 + tabbyAPI
	// the model otherwise spends huge max_tokens budget on a CoT
	// preamble that breaks the JSON gate. Harmless on llama-server
	// / GGUF (unknown body fields ignored). Applied to every pass
	// since all five emit strict JSON.
	thinkingOff := map[string]any{"enable_thinking": false}

	seedEras := p.Text("seed_eras").
		Capability("long_form").
		PromptFS(PromptsFS, "timeline_seed_eras.md").
		OutputFormatJSON().
		Output("eras.json").
		Param("temperature", 0.7).
		Param("max_tokens", 4096).
		Param("chat_template_kwargs", thinkingOff).
		Retry(jsonRetry)

	seedAnchors := p.Text("seed_anchors").
		Capability("long_form").
		After(seedEras).
		PromptFS(PromptsFS, "timeline_seed_anchors.md").
		OutputFormatJSON().
		Output("anchors.json").
		Param("temperature", 0.6).
		Param("max_tokens", 16384).
		Param("chat_template_kwargs", thinkingOff).
		Retry(jsonRetry)

	elaborateRegional := p.Text("elaborate_regional").
		Capability("long_form").
		After(seedEras, seedAnchors).
		PromptFS(PromptsFS, "timeline_elaborate_regional.md").
		OutputFormatJSON().
		Output("regional.json").
		Param("temperature", 0.7).
		Param("max_tokens", 16384).
		Param("chat_template_kwargs", thinkingOff).
		Retry(jsonRetry)

	personalise := p.Text("personalise").
		Capability("long_form").
		After(seedEras, seedAnchors, elaborateRegional).
		PromptFS(PromptsFS, "timeline_personalise.md").
		OutputFormatJSON().
		Output("personal.json").
		Param("temperature", 0.8).
		Param("max_tokens", 12288).
		Param("chat_template_kwargs", thinkingOff).
		Retry(jsonRetry)

	p.Text("fog_pass").
		Capability("long_form").
		After(seedEras, seedAnchors, elaborateRegional, personalise).
		PromptFS(PromptsFS, "timeline_fog_pass.md").
		OutputFormatJSON().
		Output("visibilities.json").
		Param("temperature", 0.3).
		Param("max_tokens", 24576).
		Param("chat_template_kwargs", thinkingOff).
		Retry(jsonRetry)

	return p.Build()
}

// jsonStageOutput is a tiny helper documenting the per-pass on-disk
// shape so the CLI's merge logic stays readable. Each pass emits one
// of these into its named output file under the run dir.
type jsonStageOutput struct {
	Path string // e.g. "eras.json"
	What string // human-readable label
}

// TimelineGenOutputs lists every file the pipeline produces. Useful
// to the CLI's post-pipeline merge step.
var TimelineGenOutputs = []jsonStageOutput{
	{Path: "eras.json", What: "named eras"},
	{Path: "anchors.json", What: "anchor events"},
	{Path: "regional.json", What: "regional consequences"},
	{Path: "personal.json", What: "personal-scale events"},
	{Path: "visibilities.json", What: "visibility classifications"},
}

// ErrTimelineGenIncomplete is returned by post-pipeline merge code
// when one of the expected output files is missing — usually
// because an earlier stage failed and a downstream one skipped.
var ErrTimelineGenIncomplete = fmt.Errorf("timeline-gen pipeline did not produce every expected output")
