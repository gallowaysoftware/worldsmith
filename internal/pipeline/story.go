package pipeline

import (
	"fmt"
	"os"
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
	// CanonRelevantFile is the path to the relevance-filtered canon
	// view for this installment — the subset of canon.md scored
	// against the brief, with world rules and on-stage-actor facts
	// always kept. The outline + writer stages read this instead of
	// the full canon so a long-running series doesn't dump every
	// fact ever recorded into the prose prompts. When empty it
	// defaults to CanonFile (the full canon), so small worlds and
	// callers that don't compute a filtered view still work.
	CanonRelevantFile string
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
	// DraftFile, when non-empty, is the path to a pre-written stitched draft
	// produced by the per-scene authoring flow. When set, the write_story stage
	// emits this file verbatim through a render instead of generating the prose in
	// one pass, so the scene-by-scene draft flows through the same expand →
	// critique → edit → narration machinery downstream unchanged.
	DraftFile string
	// OutlineJSON, when non-empty, is a pre-selected scene plan
	// (chosen by the CLI's candidate-rerank step) that replaces the
	// generated outline_story stage. The pipeline emits it verbatim
	// so the writer builds on the chosen plan. Empty (the default)
	// means generate the outline in-pipeline as usual.
	OutlineJSON string
	// NotebookFile is the path to the assembled author's notebook — the
	// private dossiers (secrets, where-it's-going, deep interiority) the
	// outline + writer draw on for depth and foreshadowing, under fog-of-war
	// (they must not reveal what the dossier marks secret). os.DevNull /
	// empty when the world has no notebook yet.
	NotebookFile string
	// LicensedRevealsFile is the path to the rendered licensed-reveals
	// allow-list for this installment (world.WriteLicensedReveals): the
	// sealed notebook material the brief permits onto the page. The
	// writer reads it to know what it MAY reveal; fog_precheck +
	// fog_check read it as the allow-list. os.DevNull / empty = nothing
	// licensed (strictest fog).
	LicensedRevealsFile string
	// NarratorVoice is the Kokoro voice id to use. Default
	// "am_fenrir" (warm baritone) — overridden via the CLI flag.
	NarratorVoice string
	// KokoroURL is the OpenAI-compatible TTS endpoint. Default
	// http://127.0.0.1:8880 (the tts_kokoro vibe service profile).
	KokoroURL string
	// SkipFinalize, when true, omits the cover-image + final-mix stages
	// from BuildStory so the CLI can run them as a separate VRAM phase
	// (BuildEpisodeFinalize) with the LLM unloaded — necessary because a
	// large resident LLM (e.g. the 28GB EXL3) leaves ComfyUI no room for
	// the cover model on a 32GB card. compose_cover still runs (it's the
	// LLM-written prompt), producing cover_prompt.txt for the finalize phase.
	SkipFinalize bool
	// CoverPromptFile is the path to cover_prompt.txt (produced by phase 1)
	// that BuildEpisodeFinalize reads to render the cover. Set by the CLI.
	CoverPromptFile string
	// SkipNarration, when true, stops BuildStory after the prose is edited
	// and the terminal continuity/fog checks have run — it omits narration
	// (showrunner/TTS/mix), canon extraction, and finalize. The CLI uses this
	// for the verify-loop: produce candidate prose + its check reports, decide
	// whether to run a targeted fix, and only narrate once the prose has
	// converged (so the slow TTS pass never runs on prose that would fail).
	SkipNarration bool
	// PreEdited, when true, makes edit_story a pass-through of the supplied
	// DraftFile instead of an LLM rewrite — the draft is already edited (it
	// came out of the verify-loop's polish_fix pass). Downstream stages still
	// read .stages.edit_story.output, so narration/canon/checks run on the
	// converged prose unchanged. Requires DraftFile.
	PreEdited bool
	// TargetWords sets the installment's target prose length, threaded to the
	// outline's per-scene budgets. 0 = pipeline default (~10,000). The series
	// flow sets it per book (~5,500/chapter).
	TargetWords int
	// ChapterFactsFile is the path to the per-chapter grounding fact-sheet
	// (BuildChapterFacts output) — the exact canon/mechanics this chapter's
	// events touch, pinned into the writer so it doesn't improvise them.
	// Empty/DevNull when none.
	ChapterFactsFile string
	// Seed, when non-zero, varies the per-scene writer's sampling seed so a
	// best-of-N attempt generates fresh prose instead of the cache's byte-identical
	// replay. 0 = unseeded (single-pass default).
	Seed int
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
//	continuity_check (text) → continuity_report.md — contradiction audit
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
	if cfg.CanonRelevantFile == "" {
		cfg.CanonRelevantFile = cfg.CanonFile
	}
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	if cfg.LicensedRevealsFile == "" {
		cfg.LicensedRevealsFile = os.DevNull
	}

	p := vamp.New("worldsmith-story").
		Describe("Generate one installment of a serialised work of fiction: prose → audiobook.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md (immutable bible)."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md (may be empty for installment 1)."))
	p.Input("canon_relevant_file", vamp.WithDefault(cfg.CanonRelevantFile),
		vamp.Describe("Path to the relevance-filtered canon view (full canon when small). Outline + writer read this; canon_delta + continuity read the full canon_file."))
	p.Input("priors_file", vamp.Required(), vamp.WithDefault(cfg.PriorsFile),
		vamp.Describe("Path to concatenated prior-installment summaries (may be empty for #1)."))
	p.Input("brief_file", vamp.Required(), vamp.WithDefault(cfg.BriefFile),
		vamp.Describe("Path to this installment's brief.md (human-authored direction)."))
	p.Input("historical_context_file", vamp.WithDefault(cfg.HistoricalContextFile),
		vamp.Describe("Path to the pre-filtered timeline view for this installment (rendered by the CLI before pipeline kickoff). Empty file when the world has no timeline."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the assembled author's notebook (private dossiers). Empty/DevNull when none."))
	p.Input("licensed_reveals_file", vamp.WithDefault(cfg.LicensedRevealsFile),
		vamp.Describe("Path to the rendered licensed-reveals allow-list (sealed material the brief permits on the page). Empty/DevNull = nothing licensed."))
	p.Input("draft_file", vamp.WithDefault(cfg.DraftFile),
		vamp.Describe("Path to a pre-written stitched draft (per-scene authoring flow). When set, write_story emits it verbatim instead of generating prose."))
	p.Input("installment_number", vamp.WithDefault(fmt.Sprintf("%d", cfg.InstallmentNumber)),
		vamp.Describe("1-indexed installment number; baked into m4b metadata."))
	p.Input("narrator_voice", vamp.WithDefault(cfg.NarratorVoice),
		vamp.Describe("Kokoro voice for narrator/description segments; also the fallback when a routed host is unknown."))

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
	var outline vamp.Ref
	if cfg.OutlineJSON != "" {
		// A pre-selected outline (the candidate-rerank step upstream
		// generated several and chose this one). Emit it verbatim
		// through a render stage so the writer reads the chosen plan
		// via .stages.outline_story.output exactly as it would a
		// freshly-generated one — no other stage needs to change.
		outline = p.Render("outline_story").
			Prompt(cfg.OutlineJSON).
			OutputFormatJSON().
			Output("outline.json")
	} else {
		outline = p.Text("outline_story").
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
	}

	var draft vamp.Ref
	if cfg.DraftFile != "" {
		// Per-scene authoring: the CLI generated the draft scene by scene and
		// stitched it; emit it verbatim so downstream reads .stages.write_story.output
		// exactly as it would a single-pass draft.
		draft = p.Render("write_story").
			Prompt(`{{ readFile .inputs.draft_file }}`).
			Output("draft.md")
	} else {
		draft = p.Text("write_story").
			Capability("long_form").
			After(outline).
			PromptFS(PromptsFS, "write_story.md").
			Output("draft.md").
			Param("temperature", 0.8).
			// 24k token budget — fits 7-9k words of prose with headroom.
			Param("max_tokens", 24576).
			Param("chat_template_kwargs", thinkingOff).
			Param("repetition_penalty", 1.1).
			Param("presence_penalty", 0.3).
			// min_p: EXL3/tabbyAPI needs an explicit min_p or long output can
			// collapse into repetition (GGUF tolerates its absence); 0.05 prunes
			// the degenerate tail without flattening creative range at temp 0.8.
			Param("min_p", 0.05)
	}

	// Prose quality runs as a diagnose-then-fix cycle ported from the short-form
	// mill, AFTER the length pass — because the length pass's "write more / trim"
	// mandate drowns out "cut this slop" notes if you ask it to do both at once.
	// So: expand_story owns LENGTH (expand/extend/polish/trim by word count);
	// prose_critique then NAMES the remaining problems (slop, "not X but Y",
	// narrated subtext, rule-breaks, continuity slips, withheld resolved early),
	// quoting each; edit_story applies that concrete list SURGICALLY with no length
	// mandate, so the fixes actually land. edit_story stays the final prose stage
	// (downstream reads .stages.edit_story.output unchanged).

	// Length pass: bring the draft to audiobook length (the former edit_story).
	var expanded vamp.Ref
	if cfg.DraftFile != "" {
		// Per-scene authoring already fixes length: each scene is written to its
		// own word budget and the budgets sum to the installment target. A
		// length-targeting LLM pass here does not help and actively fights that
		// control — in testing it crushed a 12.5k-word stitched draft to 6.3k.
		// So in the per-scene flow expand_story is a pass-through; prose_critique
		// + edit_story still run, and edit_story is held to the input's length
		// (see edit_story.md), so the per-scene budgets decide the final length.
		expanded = p.Render("expand_story").
			After(draft).
			Prompt(`{{ .stages.write_story.output }}`).
			Output("story_expanded.md")
	} else {
		expanded = p.Text("expand_story").
			Capability("long_form").
			After(draft).
			PromptFS(PromptsFS, "expand_story.md").
			Output("story_expanded.md").
			Param("temperature", 0.4).
			Param("max_tokens", 24576).
			Param("chat_template_kwargs", thinkingOff).
			Param("repetition_penalty", 1.1).
			Param("presence_penalty", 0.3).
			// min_p: EXL3/tabbyAPI needs an explicit min_p or long output can
			// collapse into repetition (GGUF tolerates its absence); 0.05 prunes
			// the degenerate tail without flattening creative range at temp 0.8.
			Param("min_p", 0.05)
	}

	// In PreEdited mode the supplied draft already went through the verify-loop's
	// polish_fix pass, so edit_story is a pass-through and the diagnosis stages are
	// skipped entirely (no second rewrite, no wasted LLM calls on the re-verify runs).
	var edited vamp.Ref
	if cfg.PreEdited {
		edited = p.Render("edit_story").
			After(expanded).
			Prompt(`{{ .stages.expand_story.output }}`).
			Output("story.md")
	} else {
		// Diagnosis: a separate cool pass names every remaining problem, quoting it.
		// Models diagnose violations reliably even when a blind rewrite won't avoid
		// them, so naming first makes the fix concrete.
		critiqued := p.Text("prose_critique").
			Capability("long_form").
			After(outline, expanded). // needs outline_story.output (budgets/shifts) + the prose
			PromptFS(PromptsFS, "prose_critique.md").
			Output("prose_critique.md").
			Param("temperature", 0.2).
			Param("max_tokens", 8192).
			Param("chat_template_kwargs", thinkingOff)

		// Continuity PRE-check: a dedicated low-temperature pass reads the prose against
		// the full canon + the bible's binding Rules and names every rule-break /
		// contradiction. Unlike the terminal continuity_check (report-only, below), this
		// one FEEDS the edit so hard-magic cheats get fixed, not just logged — closing
		// the gap where the world's core promise ("the hard magic is never cheated")
		// could ship violated.
		continuityPre := p.Text("continuity_precheck").
			Capability("long_form").
			After(expanded).
			PromptFS(PromptsFS, "continuity_precheck.md").
			Output("continuity_precheck.md").
			Param("temperature", 0.1).
			Param("max_tokens", 8192).
			Param("chat_template_kwargs", thinkingOff)

		// Fog-of-war PRE-check: a dedicated low-temperature pass reads the prose against
		// the sealed notebook, the canon ledger (what is already revealed), and the
		// brief's licensed reveals, and names every place the prose STATES sealed or
		// never-reveal material the brief did not license — quoting each. Like
		// continuity_precheck it FEEDS edit_story, so leaks are surgically removed before
		// ship: the mechanical guard on top of the writer's fog discipline. This is the
		// load-bearing protection for a world with a deep notebook (a leak the writer
		// makes would otherwise be canonised by canon_delta, cementing the spoiler).
		fogPre := p.Text("fog_precheck").
			Capability("long_form").
			After(expanded).
			PromptFS(PromptsFS, "fog_precheck.md").
			Output("fog_precheck.md").
			Param("temperature", 0.1).
			Param("max_tokens", 8192).
			Param("chat_template_kwargs", thinkingOff)

		// Surgical fix: apply every prose note AND every continuity finding AND every
		// fog-of-war leak, change nothing else, hold the length.
		edited = p.Text("edit_story").
			Capability("long_form").
			After(expanded, critiqued, continuityPre, fogPre).
			PromptFS(PromptsFS, "edit_story.md").
			Output("story.md").
			Param("temperature", 0.4).
			Param("max_tokens", 24576).
			Param("chat_template_kwargs", thinkingOff).
			Param("repetition_penalty", 1.1).
			Param("presence_penalty", 0.3).
			// min_p: EXL3/tabbyAPI needs an explicit min_p or long output can
			// collapse into repetition (GGUF tolerates its absence); 0.05 prunes
			// the degenerate tail without flattening creative range at temp 0.8.
			Param("min_p", 0.05)
	}

	// Terminal checks on the edited prose. They both FEED the CLI verify-loop
	// (which decides whether a targeted polish_fix pass is needed) and land beside
	// the m4b as audit reports. Defined here, before narration, so a SkipNarration
	// run returns the candidate prose + its reports without paying for TTS/mix.
	//
	// NOTE: these checks deliberately do NOT use OutputFormatJSON. A check is
	// informational — if the model emits malformed JSON (e.g. a raw newline inside a
	// string), it must NOT error the stage and kill a 30-minute generation run. The
	// prompt asks for JSON-only and the consumers (verdictCount, the scorecard's
	// countFindingsJSON) strip a code fence and try-JSON-then-regex, degrading an
	// unparseable report to "no findings" rather than crashing. Graceful > strict here.
	checkRetry := &vamp.RetryPolicy{
		MaxAttempts:    2,
		InitialBackoff: 5 * time.Second,
		MaxBackoff:     20 * time.Second,
		RetryOn:        []string{"transient"},
	}
	p.Text("continuity_check").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "continuity_check.md").
		Output("continuity_report.md").
		Param("temperature", 0.1).
		// Headroom + the "one short sentence per field" prompt rule keep the JSON whole.
		Param("max_tokens", 24576).
		Param("chat_template_kwargs", thinkingOff).
		Retry(checkRetry)

	p.Text("fog_check").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "fog_check.md").
		Output("fog_report.md").
		Param("temperature", 0.1).
		Param("max_tokens", 24576).
		Param("chat_template_kwargs", thinkingOff).
		Retry(checkRetry)

	// Verify-loop exit: stop after the candidate prose + its check reports so the
	// CLI can decide whether to run a targeted fix before committing to narration.
	if cfg.SkipNarration {
		return p.Build()
	}

	// ---- canon + summary (run in parallel with cover) ----

	// Raw extraction: pull the installment's new atomic facts from the prose.
	canonRaw := p.Text("canon_delta").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "canon_delta.md").
		Output("canon_delta_raw.md").
		Param("temperature", 0.2).
		Param("max_tokens", 8192).
		Param("chat_template_kwargs", thinkingOff)

	summarized := p.Text("summarize").
		Capability("long_form").
		After(edited).
		PromptFS(PromptsFS, "summarize.md").
		Output("summary.md").
		Param("temperature", 0.3).
		Param("max_tokens", 4096).
		Param("chat_template_kwargs", thinkingOff)

	// Canon RECONCILIATION: extraction can drift — record a death the prose
	// didn't show, a mechanism the §1 fix replaced, an invented second surgeon,
	// or a fact that contradicts prior canon. This pass corrects the raw delta
	// against the FINISHED PROSE (source of truth), the summary, the existing
	// canon, and the bible/characters, so the canon the next installment reads
	// can never diverge from what actually shipped. Its output is canon_delta.md
	// — the file the CLI folds into canon.md.
	p.Text("reconcile_canon").
		Capability("long_form").
		After(edited, canonRaw, summarized).
		PromptFS(PromptsFS, "reconcile_canon.md").
		Output("canon_delta.md").
		Param("temperature", 0.2).
		Param("max_tokens", 8192).
		Param("chat_template_kwargs", thinkingOff)

	// (continuity_check + fog_check now run earlier, right after edit_story, so the
	// verify-loop and the SkipNarration exit can use their reports.)

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

	// ---- narration script ----

	// Number the edited prose's paragraphs deterministically; the showrunner then
	// routes each by index (voice only, no text echo). This keeps showrunner's output
	// tiny and fast even for a 12k-word installment — echoing the whole prose into one
	// JSON blew past the 32k-token cap and truncated. The text is rejoined to the
	// routing by index in enumerate_segments.
	numberParas := p.Render("number_paragraphs").
		After(edited).
		Prompt(`{"items": {{ chunkParagraphs .stages.edit_story.output 1 }} }`).
		Output("paragraphs.json").
		OutputFormatJSON()

	showrunner := p.Text("showrunner").
		Capability("long_form").
		After(numberParas).
		PromptFS(PromptsFS, "showrunner.md").
		OutputFormatJSON().
		Output("script.json").
		Param("temperature", 0.2).
		// Route-only output is one compact entry per paragraph; a long, dialogue-heavy
		// installment can run to ~450 paragraphs (~10k tokens of routing), so the cap
		// must clear that. 24576 covers ~1,000 paragraphs; the model stops at the JSON
		// close well before it.
		Param("max_tokens", 24576).
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
		}).
		// showrunner is the LAST LLM stage in the pipeline — everything after it
		// (enumerate_segments, cast_voice/TTS, the mix) is render/audio/ffmpeg and
		// needs no LLM. Unload the ~28GB long_form model the moment its group is
		// done so it isn't left resident on the card WHILE Kokoro renders narration:
		// LLM(28GB)+TTS(6GB) on a 32GB card oversubscribed VRAM and hard-locked the
		// box mid-narration (the overnight-novel freeze). Group-level free waits for
		// reconcile_canon (showrunner's wave-mate) too, so nothing is cut off; the
		// next chapter's prose pass re-activates the LLM on demand.
		FreeProfileAfter()

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
	// Rejoin: walk the numbered paragraphs (authoritative text + order), look up each
	// one's routing from the showrunner by position (it emits one entry per paragraph
	// in order), split long paragraphs at sentence boundaries, and assign final IDs.
	// Paragraphs with no routing entry fall back to the narrator.
	segments := p.Render("enumerate_segments").
		After(numberParas, showrunner).
		Prompt(`{"items": [
{{- $paras := index (parseJSON .stages.number_paragraphs.output) "items" -}}
{{- $routes := index (parseJSON .stages.showrunner.output) "segments" -}}
{{- $chars := index (parseJSON (readFile .inputs.characters_file)) "characters" -}}
{{- $narrator := .inputs.narrator_voice -}}
{{- $i := 0 -}}
{{- $first := true -}}
{{- range $pi, $para := $paras -}}
{{- $text := trim (index $para "text") -}}
{{- if and (ne $text "") (ne $text "***") (ne $text "---") (ne $text "* * *") -}}
  {{- $host := "narrator" -}}
  {{- if lt $pi (len $routes) -}}
    {{- $host = index (index $routes $pi) "host" -}}
  {{- end -}}
  {{/* Derive the voice from the host slug + the authoritative cast file —
       never the showrunner's echoed voice_id, which can be typo'd
       (e.g. "am_fenfir") and 400 the TTS call. narrator and any unknown
       host both fall back to the narrator voice. */}}
  {{- $voice := $narrator -}}
  {{- if ne $host "narrator" -}}
    {{- range $c := $chars -}}
      {{- if eq (index $c "slug") $host -}}{{- $voice = index $c "voice_id" -}}{{- end -}}
    {{- end -}}
  {{- end -}}
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
		After(segments, audio).
		Prompt(`{{ $script := parseJSON .stages.enumerate_segments.output -}}
{"voice_segments": [
{{- range $i, $seg := index $script "items" -}}
  {{- if $i }}, {{ end -}}
  "audio/{{ index $seg "id" }}.wav"
{{- end }}
]}`).
		Output("mix_script.json").
		OutputFormatJSON()

	// The cover IMAGE and final mix run as a separate VRAM phase (the LLM unloaded so
	// ComfyUI gets the card) when the CLI sets SkipFinalize — see BuildEpisodeFinalize.
	// compose_cover (above) already produced cover_prompt.txt for that phase.
	if cfg.SkipFinalize {
		return p.Build()
	}

	// Single-pass path (doctor/activate stub, or a caller whose LLM is small enough to
	// leave the image model room): render the cover + mix inline.
	generateCover := p.ComfyUI("generate_cover").
		Capability("image_gen").
		After(composeCover).
		WorkflowFS(WorkflowsFS, "sdxl_turbo.json").
		Parameter("4.text", "{{ .stages.compose_cover.output }}").
		Parameter("6.width", "1024").
		Parameter("6.height", "1024").
		FreeMemoryAfter(). // unload the image model after the cover so the next run's LLM has VRAM
		Output("cover.png")

	p.Mix("mix_episode").
		After(mixScript, audio, generateCover).
		ScriptFile("mix_script.json").
		CoverImage("cover.png").
		LoudnessTarget(-18). // R128 audiobook standard (vs -16 for podcasts)
		Metadata("title", fmt.Sprintf("Installment %d", cfg.InstallmentNumber)).
		Metadata("artist", "worldsmith narrator").
		Metadata("genre", "Audiobook").
		Metadata("track", fmt.Sprintf("%d", cfg.InstallmentNumber)).
		Output("episode.m4b")

	return p.Build()
}

// PolishFixConfig drives one targeted fix pass in the verify-loop. It reads the
// current prose plus the terminal continuity and fog reports and rewrites ONLY the
// flagged spans, length-preserving — the narrow, reliable alternative to asking the
// omnibus edit pass to catch everything. The CLI runs it between a check and a
// re-check, looping until the prose is clean (or a cap is hit).
type PolishFixConfig struct {
	ProseFile            string
	FogReportFile        string
	ContinuityReportFile string
	WorldFile            string
	CharactersFile       string
	CanonFile            string
	NotebookFile         string
	LicensedRevealsFile  string
	BriefFile            string
	// OutputName is the rendered prose filename in the run-dir; defaults to
	// "polished.md".
	OutputName string
}

// BuildPolishFix constructs the one-stage pipeline that applies the flagged
// continuity/fog fixes to the prose and nothing else.
func BuildPolishFix(cfg PolishFixConfig) (*vamp.Pipeline, error) {
	if cfg.OutputName == "" {
		cfg.OutputName = "polished.md"
	}
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	if cfg.LicensedRevealsFile == "" {
		cfg.LicensedRevealsFile = os.DevNull
	}
	if cfg.CanonFile == "" {
		cfg.CanonFile = os.DevNull
	}

	p := vamp.New("worldsmith-polish-fix").
		Describe("Apply only the flagged continuity/fog fixes to the prose, length-preserving.")

	p.Input("prose_file", vamp.Required(), vamp.WithDefault(cfg.ProseFile),
		vamp.Describe("Path to the current prose to fix."))
	p.Input("fog_report_file", vamp.WithDefault(cfg.FogReportFile),
		vamp.Describe("Path to fog_report.md (the leaks to un-name)."))
	p.Input("continuity_report_file", vamp.WithDefault(cfg.ContinuityReportFile),
		vamp.Describe("Path to continuity_report.md (the contradictions to fix)."))
	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md (the revealed-to-reader ledger)."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the sealed author's notebook."))
	p.Input("licensed_reveals_file", vamp.WithDefault(cfg.LicensedRevealsFile),
		vamp.Describe("Path to the licensed-reveals allow-list."))
	p.Input("brief_file", vamp.Required(), vamp.WithDefault(cfg.BriefFile),
		vamp.Describe("Path to this installment's brief.md."))

	p.RequireProfile("long_form")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("polish_fix").
		Capability("long_form").
		PromptFS(PromptsFS, "polish_fix.md").
		Output(cfg.OutputName).
		Param("temperature", 0.3).
		Param("max_tokens", 24576).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Param("repetition_penalty", 1.1).
		Param("presence_penalty", 0.3).
		Param("min_p", 0.05).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	return p.Build()
}

// SpanFixConfig drives the span-level fix pass — the reliable replacement for the
// whole-document polish_fix (which the model treated as "reproduce the doc" and applied
// nothing). It feeds the model ONLY the flagged spans (from the fog/continuity reports),
// so the model produces short replacements instead of copying 10k words; the CLI then
// splices each replacement into the prose deterministically.
type SpanFixConfig struct {
	FogReportFile        string
	ContinuityReportFile string
	WorldFile            string
	NotebookFile         string
	LicensedRevealsFile  string
	BriefFile            string
	// OutputName is the rendered replacements filename; defaults to "spanfix.json".
	OutputName string
}

// BuildSpanFix constructs the one-stage pipeline that returns a JSON list of
// {span, replacement} for every flagged span — the model never sees the full prose.
func BuildSpanFix(cfg SpanFixConfig) (*vamp.Pipeline, error) {
	if cfg.OutputName == "" {
		cfg.OutputName = "spanfix.json"
	}
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	if cfg.LicensedRevealsFile == "" {
		cfg.LicensedRevealsFile = os.DevNull
	}

	p := vamp.New("worldsmith-span-fix").
		Describe("Produce replacement text for each flagged span (continuity + fog), for deterministic splicing.")

	p.Input("fog_report_file", vamp.WithDefault(cfg.FogReportFile),
		vamp.Describe("Path to fog_report.md (JSON leaks)."))
	p.Input("continuity_report_file", vamp.WithDefault(cfg.ContinuityReportFile),
		vamp.Describe("Path to continuity_report.md (JSON contradictions)."))
	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the sealed author's notebook."))
	p.Input("licensed_reveals_file", vamp.WithDefault(cfg.LicensedRevealsFile),
		vamp.Describe("Path to the licensed-reveals allow-list."))
	p.Input("brief_file", vamp.Required(), vamp.WithDefault(cfg.BriefFile),
		vamp.Describe("Path to this installment's brief.md."))

	p.RequireProfile("long_form")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("span_fix").
		Capability("long_form").
		PromptFS(PromptsFS, "span_fix.md").
		Output(cfg.OutputName).
		OutputFormatJSON().
		Param("temperature", 0.3).
		Param("max_tokens", 8192).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Param("min_p", 0.05).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	return p.Build()
}

// ProsePolishConfig drives the style-polish pass: recast a specific set of
// flagged sentences (over-used openers, slop terms, the not-X-but-Y cadence)
// without changing meaning or length. Like BuildSpanFix it feeds the model only
// the flagged spans — never the whole document — so the model can't lapse into
// reproduce-the-doc or silently compress length; the CLI splices each
// replacement back by exact substring.
type ProsePolishConfig struct {
	// SpansFile is the path to the JSON list of offending sentences
	// ({"sentences":[{"span","reason"}]}) produced by world.OffendingSentences.
	SpansFile string
	// NotebookFile + LicensedRevealsFile give the pass the SAME fog-of-war
	// context the writer has, so a recast can't turn an oblique line into a
	// sealed-material leak (the failure mode without them). Empty/DevNull when
	// the world has no notebook / nothing licensed.
	NotebookFile        string
	LicensedRevealsFile string
	// OutputName is the rendered replacements filename; defaults to
	// "prose_polish.json".
	OutputName string
}

// BuildProsePolish constructs the one-stage pipeline that returns a JSON list of
// {span, replacement} recasts for each flagged sentence — the per-scene flow's
// missing style remediation (edit_story is a pass-through in PreEdited mode, so
// slop/anaphora otherwise ship unfixed).
func BuildProsePolish(cfg ProsePolishConfig) (*vamp.Pipeline, error) {
	if cfg.OutputName == "" {
		cfg.OutputName = "prose_polish.json"
	}
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	if cfg.LicensedRevealsFile == "" {
		cfg.LicensedRevealsFile = os.DevNull
	}
	p := vamp.New("worldsmith-prose-polish").
		Describe("Recast flagged sentences (repeated openers, slop, not-X-but-Y) without changing meaning, length, or leaking sealed material.")

	p.Input("spans_file", vamp.Required(), vamp.WithDefault(cfg.SpansFile),
		vamp.Describe("Path to the JSON list of offending sentences ({sentences:[{span,reason}]})."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the assembled author's notebook (sealed material; fog of war)."))
	p.Input("licensed_reveals_file", vamp.WithDefault(cfg.LicensedRevealsFile),
		vamp.Describe("Path to the licensed-reveals allow-list (sealed material the brief permits on the page)."))

	p.RequireProfile("long_form")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("prose_polish").
		Capability("long_form").
		PromptFS(PromptsFS, "prose_polish.md").
		Output(cfg.OutputName).
		OutputFormatJSON().
		Param("temperature", 0.5).
		// Up to ~40 sentence rewrites + JSON scaffolding fits comfortably.
		Param("max_tokens", 12288).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Param("min_p", 0.05).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	return p.Build()
}

// ContinuityVerifyConfig configures the adversarial false-positive audit of the
// continuity findings (see BuildContinuityVerify).
type ContinuityVerifyConfig struct {
	ContinuityReportFile string
	WorldFile            string
	CanonFile            string
	OutputName           string // default "continuity_verify.json"
}

// BuildContinuityVerify constructs the one-stage pipeline that audits each continuity
// finding for false positives: it returns, per finding, a REAL/FALSE verdict plus the
// verbatim bible/canon sentence the span supposedly contradicts. The CLI then drops any
// finding that is FALSE or whose cited quote does not actually occur in the bible/canon —
// so a real break must be backed by real established text, killing the checker's
// absence-is-not-prohibition over-flags and its self-negating non-findings.
func BuildContinuityVerify(cfg ContinuityVerifyConfig) (*vamp.Pipeline, error) {
	if cfg.OutputName == "" {
		cfg.OutputName = "continuity_verify.json"
	}
	p := vamp.New("worldsmith-continuity-verify").
		Describe("Audit continuity findings for false positives; demand a verbatim contradicted canon line.")

	p.Input("continuity_report_file", vamp.Required(), vamp.WithDefault(cfg.ContinuityReportFile),
		vamp.Describe("Path to continuity_report.md (JSON findings to audit)."))
	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md."))

	p.RequireProfile("long_form")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("continuity_verify").
		Capability("long_form").
		PromptFS(PromptsFS, "continuity_verify.md").
		Output(cfg.OutputName).
		Param("temperature", 0.1).
		Param("max_tokens", 4096).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Param("min_p", 0.05).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    2,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     20 * time.Second,
			RetryOn:        []string{"transient"},
		})

	return p.Build()
}

// BuildEpisodeFinalize is phase 2 of the story flow: render the cover image and mix
// the final m4b. It is a SEPARATE pipeline so the CLI can unload the LLM first — a
// 28GB EXL3 leaves a 32GB card no room for the cover model. It reads phase-1 artifacts
// from the same run-dir: cover_prompt.txt (via CoverPromptFile), mix_script.json, and
// the audio/ wavs; it writes cover.png + episode.m4b.
func BuildEpisodeFinalize(cfg StoryConfig) (*vamp.Pipeline, error) {
	p := vamp.New("worldsmith-finalize").
		Describe("Cover image + final mix for an installment (runs with the LLM unloaded so ComfyUI gets the VRAM).")

	p.Input("cover_prompt_file", vamp.Required(), vamp.WithDefault(cfg.CoverPromptFile),
		vamp.Describe("Path to cover_prompt.txt produced by phase 1 (compose_cover)."))
	p.RequireService("comfyui", "http://127.0.0.1:8188",
		"ComfyUI — SDXL cover-art generation.", "vibe start comfyui")
	p.RequireGPUMemory("~4GB (SDXL cover) — run with the LLM unloaded")

	// Load the phase-1 cover prompt off disk into a stage output (Render supports
	// readFile — more robust than a readFile inside a ComfyUI parameter template).
	coverPrompt := p.Render("load_cover_prompt").
		Prompt(`{{ readFile .inputs.cover_prompt_file }}`).
		Output("cover_prompt_loaded.txt")

	generateCover := p.ComfyUI("generate_cover").
		Capability("image_gen").
		After(coverPrompt).
		WorkflowFS(WorkflowsFS, "sdxl_turbo.json").
		Parameter("4.text", "{{ .stages.load_cover_prompt.output }}").
		Parameter("6.width", "1024").
		Parameter("6.height", "1024").
		FreeMemoryAfter(). // unload the image model after the cover so the next run's LLM has VRAM
		Output("cover.png")

	// mix_script.json + audio/*.wav already exist in the run-dir from phase 1.
	p.Mix("mix_episode").
		After(generateCover).
		ScriptFile("mix_script.json").
		CoverImage("cover.png").
		LoudnessTarget(-18).
		Metadata("title", fmt.Sprintf("Installment %d", cfg.InstallmentNumber)).
		Metadata("artist", "worldsmith narrator").
		Metadata("genre", "Audiobook").
		Metadata("track", fmt.Sprintf("%d", cfg.InstallmentNumber)).
		Output("episode.m4b")

	return p.Build()
}
