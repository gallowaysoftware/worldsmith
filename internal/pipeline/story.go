package pipeline

import (
	"fmt"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// StoryConfig drives one installment's generation. The CLI binary
// populates these from the world layout + the current installment
// number.
type StoryConfig struct {
	// InstallmentNumber is the 1-indexed position in the series.
	InstallmentNumber int
	// WorldFile is the path to world.md (the immutable bible).
	WorldFile string
	// CharactersFile is the path to characters.json.
	CharactersFile string
	// CanonFile is the path to canon.md (auto-grown across
	// installments). Must exist; may be empty for installment 1.
	CanonFile string
	// PriorsFile is the path to the concatenated prior-installment
	// summaries. Must exist; empty for installment 1.
	PriorsFile string
	// BriefFile is the path to this installment's brief.md (the
	// per-installment human direction).
	BriefFile string
	// NarratorVoice is the Kokoro voice id to use. Default
	// "am_fenrir" (warm baritone) — overridden via the CLI flag.
	NarratorVoice string
	// KokoroURL is the OpenAI-compatible TTS endpoint. Default
	// http://127.0.0.1:8880 (the tts_kokoro vibe service profile).
	KokoroURL string
}

// BuildStory constructs the per-installment vamp pipeline for the
// `worldsmith story` flow.
//
// Stages:
//
//	write_story    (text)   → draft.md     — 5-8k word prose draft
//	edit_story     (text)   → story.md     — quality pass
//	canon_delta    (text)   → canon_delta.md — atomic facts the next
//	                                          installment must know
//	summarize      (text)   → summary.md   — short recap → priors_file
//	compose_cover  (text)   → cover_prompt.txt
//	generate_cover (comfyui) → cover.png
//	showrunner     (text/json) → script.json — paragraph segments
//	cast_voice     (audio foreach) → audio/seg_NNN.wav
//	compose_mix    (render) → mix_script.json
//	mix_episode    (mix)    → episode.m4b
//
// Inputs threaded through prompts: world_file, characters_file,
// canon_file, priors_file, brief_file (all paths the prompts'
// readFile calls resolve against).
func BuildStory(cfg StoryConfig) (*vamp.Pipeline, error) {
	if cfg.NarratorVoice == "" {
		cfg.NarratorVoice = "am_fenrir"
	}
	if cfg.KokoroURL == "" {
		cfg.KokoroURL = "http://127.0.0.1:8880"
	}

	p := vamp.New("worldsmith-story").
		Describe("Generate one installment of a serialised work of fiction: prose → audiobook.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md (immutable bible)."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md (may be empty for installment 1)."))
	p.Input("priors_file", vamp.Required(), vamp.WithDefault(cfg.PriorsFile),
		vamp.Describe("Path to concatenated prior-installment summaries (may be empty for #1)."))
	p.Input("brief_file", vamp.Required(), vamp.WithDefault(cfg.BriefFile),
		vamp.Describe("Path to this installment's brief.md (human-authored direction)."))
	p.Input("installment_number", vamp.WithDefault(fmt.Sprintf("%d", cfg.InstallmentNumber)),
		vamp.Describe("1-indexed installment number; baked into m4b metadata."))

	p.RequireProfile("long_form")
	p.RequireService("kokoro-tts", cfg.KokoroURL,
		"Kokoro-FastAPI TTS — narrator voice.",
		"vibe start tts_kokoro")
	p.RequireService("comfyui", "http://127.0.0.1:8188",
		"ComfyUI — SDXL cover-art generation.",
		"vibe start comfyui")
	p.RequireGPUMemory("~30GB during write/edit/showrunner; ~6GB during TTS; ~4GB during SDXL")
	p.RequireDiskSpace("~50MB per installment (audio + intermediate JSON)")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	// ---- prose ----

	draft := p.Text("write_story").
		Capability("long_form").
		PromptFS(PromptsFS, "write_story.md").
		Output("draft.md").
		Param("temperature", 0.8).
		// 24k token budget — accommodates a thinking-preamble (Qwen
		// emits up to 4k of <think>) plus 5-8k words ≈ 7-10k tokens
		// of actual prose, with comfortable headroom for the ending.
		Param("max_tokens", 24576)

	edited := p.Text("edit_story").
		Capability("long_form").
		After(draft).
		PromptFS(PromptsFS, "edit_story.md").
		Output("story.md").
		Param("temperature", 0.4).
		Param("max_tokens", 24576)

	// ---- canon + summary (run in parallel with cover) ----

	p.Text("canon_delta").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "canon_delta.md").
		Output("canon_delta.md").
		Param("temperature", 0.2).
		Param("max_tokens", 8192)

	p.Text("summarize").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "summarize.md").
		Output("summary.md").
		Param("temperature", 0.3).
		Param("max_tokens", 4096)

	// ---- cover ----

	composeCover := p.Text("compose_cover").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "cover.md").
		Output("cover_prompt.txt").
		Param("temperature", 0.6).
		Param("max_tokens", 4096)

	generateCover := p.ComfyUI("generate_cover").
		Capability("image_gen").
		After(composeCover).
		WorkflowFS(WorkflowsFS, "sdxl_turbo.json").
		Parameter("4.text", "{{ .stages.compose_cover.output }}").
		Parameter("6.width", "1024").
		Parameter("6.height", "1024").
		Output("cover.png")

	// ---- narration script ----

	showrunner := p.Text("showrunner").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "showrunner.md").
		OutputFormatJSON().
		Output("script.json").
		Param("temperature", 0.2).
		Param("max_tokens", 32768)

	// ---- segments + TTS ----

	segments := p.Render("enumerate_segments").
		After(showrunner).
		Prompt(`{"items": {{ toJSON (index (parseJSON .stages.showrunner.output) "segments") }} }`).
		Output("segments.json").
		OutputFormatJSON()

	audio := p.Audio("cast_voice").
		Capability("tts").
		After(segments).
		Foreach(segments, "segment").
		Engine(vamp.AudioEngineKokoro).
		EngineURL(cfg.KokoroURL).
		Voice(cfg.NarratorVoice).
		TextTemplate(`{{ ttsNormalize .segment.text "" }}`).
		Output("audio/{{.segment.id}}.wav")

	// ---- mix ----

	mixScript := p.Render("compose_mix_script").
		After(showrunner, audio).
		Prompt(`{{ $script := parseJSON .stages.showrunner.output -}}
{"voice_segments": [
{{- range $i, $seg := index $script "segments" -}}
  {{- if $i }}, {{ end -}}
  "audio/{{ index $seg "id" }}.wav"
{{- end }}
]}`).
		Output("mix_script.json").
		OutputFormatJSON()

	p.Mix("mix_episode").
		After(mixScript, audio, generateCover).
		ScriptFile("mix_script.json").
		CoverImage("cover.png").
		LoudnessTarget(-18). // R128 audiobook standard (vs -16 for podcasts)
		// Container metadata for Audiobookshelf / Plex.
		Metadata("title", fmt.Sprintf("Installment %d", cfg.InstallmentNumber)).
		Metadata("artist", "worldsmith narrator").
		Metadata("genre", "Audiobook").
		Metadata("track", fmt.Sprintf("%d", cfg.InstallmentNumber)).
		Output("episode.m4b")

	// All text stages share the DefaultTextRetry baked into vamp's
	// Text() builder — no per-stage Retry overrides needed.

	return p.Build()
}

// retryFor is the per-stage retry policy that takes longer than the
// vamp default. Kept here in case a stage needs to override (e.g.
// the write_story stage occasionally takes a few minutes per
// attempt and merits a longer initial backoff between retries).
func retryFor(maxAttempts int) *vamp.RetryPolicy {
	return &vamp.RetryPolicy{
		MaxAttempts:    maxAttempts,
		InitialBackoff: 30 * time.Second,
		MaxBackoff:     5 * time.Minute,
		RetryOn:        []string{"transient", "invalid_output"},
	}
}
