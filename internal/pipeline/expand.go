package pipeline

import (
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// ExpandConfig drives the idea-expansion pipeline: deepen ONE thread of a world
// into a private notebook dossier (what the author knows but readers haven't been
// shown). Seeded either by the human (--seed) or auto-selected from the bible's
// richest unpulled threads. The output is a PROPOSAL written to a staging dir;
// the CLI never folds it into the world without human review.
type ExpandConfig struct {
	WorldFile      string
	CharactersFile string
	CanonFile      string
	// NotebookFile is the assembled existing notebook (os.DevNull / empty file
	// when the world has none yet) — context so the room doesn't duplicate.
	NotebookFile string
	// Seed is the author's optional idea to develop. Empty = auto-select a thread.
	Seed string
	// AvoidThreads is a newline/comma list of thread titles already chosen this
	// batch, so a multi-thread run picks distinct threads.
	AvoidThreads string
}

// BuildExpand constructs the diagnose-then-fix expansion pipeline:
//
//	thread_select   (text,json) -> thread.json  — choose/formalise the thread
//	expand_room     (text)      -> room.md      — 4-lens writers' room develops it
//	expand_critique (text,json) -> critique.json — consistency/rules/specificity audit
//	expand_revise   (text)      -> dossier.md   — final dossier resolving every note
//
// The critique step exists for the same reason it does in the short-form mill:
// the model reliably DIAGNOSES contradictions and generic filler even when a
// blind draft won't avoid them. world.md is read-only throughout — expansion only
// ever produces a notebook dossier proposal.
func BuildExpand(cfg ExpandConfig) (*vamp.Pipeline, error) {
	p := vamp.New("worldsmith-expand").
		Describe("Deepen one world thread into a private notebook dossier (proposal for review).")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md (read-only; never edited)."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md."))
	p.Input("notebook_file", vamp.Required(), vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the assembled existing notebook (empty file if none)."))
	p.Input("seed", vamp.WithDefault(cfg.Seed),
		vamp.Describe("Optional author idea to develop; empty = auto-select a thread."))
	p.Input("avoid_threads", vamp.WithDefault(cfg.AvoidThreads),
		vamp.Describe("Thread titles already chosen this batch (avoid duplicates)."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during expansion")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	retry := &vamp.RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: 5 * time.Second,
		MaxBackoff:     30 * time.Second,
		RetryOn:        []string{"transient", "invalid_output"},
	}
	noThink := map[string]any{"enable_thinking": false}

	thread := p.Text("thread_select").
		Capability("long_form").
		PromptFS(PromptsFS, "thread_select.md").
		OutputFormatJSON().
		Output("thread.json").
		Param("temperature", 0.5).
		Param("max_tokens", 2048).
		Param("chat_template_kwargs", noThink).
		Retry(retry)

	room := p.Text("expand_room").
		Capability("long_form").
		After(thread).
		PromptFS(PromptsFS, "expand_room.md").
		Output("room.md").
		Param("temperature", 0.85).
		Param("max_tokens", 12288).
		Param("chat_template_kwargs", noThink).
		Retry(retry)

	critique := p.Text("expand_critique").
		Capability("long_form").
		After(thread, room).
		PromptFS(PromptsFS, "expand_critique.md").
		OutputFormatJSON().
		Output("critique.json").
		Param("temperature", 0.2).
		Param("max_tokens", 6144).
		Param("chat_template_kwargs", noThink).
		Retry(retry)

	p.Text("expand_revise").
		Capability("long_form").
		After(thread, room, critique).
		PromptFS(PromptsFS, "expand_revise.md").
		Output("dossier.md").
		Param("temperature", 0.6).
		Param("max_tokens", 10240).
		Param("chat_template_kwargs", noThink).
		Retry(retry)

	return p.Build()
}
