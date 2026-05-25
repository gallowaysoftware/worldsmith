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
	// per-installment human direction). May carry YAML frontmatter
	// (year_override, pov_region, on_stage_actors) the CLI parses
	// before kickoff to drive timeline filtering.
	BriefFile string
	// HistoricalContextFile is the path to the pre-rendered timeline
	// view for this installment (events through year_override or
	// current_year, with visibility filtering applied). Always set
	// by the CLI; empty file when the world has no timeline.json or
	// no events pass the filter.
	HistoricalContextFile string
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
	p.Input("historical_context_file", vamp.WithDefault(cfg.HistoricalContextFile),
		vamp.Describe("Path to the pre-filtered timeline view for this installment (rendered by the CLI before pipeline kickoff). Empty file when the world has no timeline."))
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

	// Shared sampler shape for the long-form prose stages. Two
	// non-defaults besides temperature:
	//   - chat_template_kwargs.enable_thinking=false: Qwen3 otherwise
	//     emits 1-4 KB of "Here's a thinking process: ..." bullets at
	//     the top of the output, which ended up *in* draft.md /
	//     story.md as visible CoT. Hard-switch off — the model still
	//     reasons, it just doesn't externalise.
	//   - repetition_penalty + presence_penalty: Qwen3 on long output
	//     (~7-8k words) at low temperature is prone to anaphora
	//     collapse near the end ("He thought of X. He thought of
	//     Y. He thought of Z." for 200+ lines). 1.1 / 0.3 is the
	//     light touch that breaks the loop without turning the prose
	//     synthetic.
	thinkingOff := map[string]any{"enable_thinking": false}

	// Outline pass: before the writer drafts, the planner turns the
	// brief's bullet-list of beats into a per-scene plan with explicit
	// word budgets, canon hooks, and turn-of-the-scene specifics. The
	// research case for this is DOC (Yang et al., ACL 2023) — a hierarchical
	// outliner stage between brief and prose raised plot coherence by
	// 22.5% in their eval. Empirically, worldsmith's earlier under-
	// length problem (002 v1 landing at 4,482 words for a 7,500-target
	// brief) was the model treating beats as summary-sized rather than
	// scene-sized; the outline forces a per-scene word budget the writer
	// has to honour.
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
		// 24k token budget — fits 7-9k words of prose with headroom.
		Param("max_tokens", 24576).
		Param("chat_template_kwargs", thinkingOff).
		Param("repetition_penalty", 1.1).
		Param("presence_penalty", 0.3)

	edited := p.Text("edit_story").
		Capability("long_form").
		After(draft).
		PromptFS(PromptsFS, "edit_story.md").
		Output("story.md").
		Param("temperature", 0.4).
		Param("max_tokens", 24576).
		Param("chat_template_kwargs", thinkingOff).
		Param("repetition_penalty", 1.1).
		Param("presence_penalty", 0.3)

	// ---- canon + summary (run in parallel with cover) ----

	p.Text("canon_delta").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "canon_delta.md").
		Output("canon_delta.md").
		Param("temperature", 0.2).
		Param("max_tokens", 8192).
		Param("chat_template_kwargs", thinkingOff)

	p.Text("summarize").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "summarize.md").
		Output("summary.md").
		Param("temperature", 0.3).
		Param("max_tokens", 4096).
		Param("chat_template_kwargs", thinkingOff)

	// ---- cover ----

	composeCover := p.Text("compose_cover").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "cover.md").
		Output("cover_prompt.txt").
		Param("temperature", 0.6).
		Param("max_tokens", 4096).
		// Same CoT-suppression as showrunner: cover.md asks for a
		// single-line SDXL prompt. Qwen3 with CoT on emitted 10KB of
		// "Thinking through the composition:" bullets instead.
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false})

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
		Param("max_tokens", 32768).
		// chat_template_kwargs.enable_thinking=false: same fix as
		// iitn's showrunner. Qwen3 emits a verbose chain-of-thought
		// preamble before strict-JSON output, which trips the JSON
		// gate with "invalid character 'T' looking for beginning of
		// value" (the prose "The script needs..." opener). Hard-
		// switching CoT off on this stage saves the run.
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 10 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	// ---- segments + TTS ----

	// Filter scene-break / empty segments and split long paragraphs
	// at sentence boundaries before TTS:
	//
	//   * Scene-break filter: showrunner occasionally emits a segment
	//     for prose markup like "***" or "---" (paragraph dividers).
	//     Kokoro fails with empty-body on those. Belt-and-braces —
	//     the showrunner prompt also rejects them, but a render-time
	//     filter ensures a single LLM slip doesn't blow up an
	//     otherwise complete pipeline.
	//
	//   * Long-segment split: Kokoro rushes calls over ~300 chars,
	//     eliding interior comma pauses. splitSentences chops long
	//     paragraphs at sentence boundaries (greedy-packed to fit
	//     under maxChars). Sub-segments inherit the parent's host /
	//     voice_id; per-call IDs are regenerated sequentially.
	//     Splitting deterministically here is more reliable than
	//     asking the showrunner to do it (we tried; the model treats
	//     "split paragraphs over 350 chars" as advisory).
	segments := p.Render("enumerate_segments").
		After(showrunner).
		Prompt(`{"items": [
{{- $segs := index (parseJSON .stages.showrunner.output) "segments" -}}
{{- $i := 0 -}}
{{- $first := true -}}
{{- range $segs -}}
{{- $text := trim (index . "text") -}}
{{- if and (ne $text "") (ne $text "***") (ne $text "---") (ne $text "* * *") -}}
  {{- $host := index . "host" -}}
  {{- $voice := index . "voice_id" -}}
  {{- $chunks := parseJSON (splitSentences $text 300) -}}
  {{- range $chunks -}}
    {{- if not $first }},
{{ end -}}{{- $first = false -}}
    {"id": "seg_{{ printf "%03d" $i }}", "host": {{ toJSON $host }}, "voice_id": {{ toJSON $voice }}, "text": {{ toJSON . }}}
    {{- $i = addInt $i 1 -}}
  {{- end -}}
{{- end -}}
{{- end -}}
] }`).
		Output("segments.json").
		OutputFormatJSON()

	// Voice is template-rendered per foreach item (vibe v0.6.2+).
	// Each segment carries its own voice_id from the showrunner —
	// narrator paragraphs get NarratorVoice (am_fenrir), named-
	// character dialogue gets that character's Kokoro voice (Tova /
	// Voss / Henr / Lis each have a voice_id in characters.json).
	// The `or` fallback keeps the stage robust to a missing voice_id
	// on a segment: empty falls through to NarratorVoice rather than
	// crashing the run.
	audio := p.Audio("cast_voice").
		Capability("tts").
		After(segments).
		Foreach(segments, "segment").
		Engine(vamp.AudioEngineKokoro).
		EngineURL(cfg.KokoroURL).
		Voice(fmt.Sprintf(`{{ or .segment.voice_id %q }}`, cfg.NarratorVoice)).
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
