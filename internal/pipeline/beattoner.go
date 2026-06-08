package pipeline

import (
	"fmt"
	"os"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// BeatTonerConfig drives the beat-toner — the LLM pass that rewrites ONE chapter's beats
// so the same plot events still happen, but the chapter's sealed material stays subtext
// instead of being dramatized on the page. It exists because some chapters leak no matter
// how the reveal is licensed or how many times the prose is re-drafted: the BEAT itself is
// built around the secret (e.g. "Augustus boasts about the bred cohort"), so the writer
// faithfully renders a leak. Toning the beat — keep the scene, move the secret off-page —
// is the only lever that helps those. Output is reviewable beats the CLI folds into arc.json.
type BeatTonerConfig struct {
	WorldFile    string
	NotebookFile string
	ChapterN     int
	Title        string
	// BeatsRendered / ConstraintsRendered / Reveals are the chapter's current beats, its
	// constraints, and its reveal-license (what MAY be shown; everything else stays sealed).
	BeatsRendered       string
	ConstraintsRendered string
	Reveals             string
	OutputName          string // default "toned_beats.json"
}

// BuildBeatToner constructs the one-stage pass. Output JSON:
// {"hook":"...","beats":["..."],"constraints":["..."]}.
func BuildBeatToner(cfg BeatTonerConfig) (*vamp.Pipeline, error) {
	if cfg.OutputName == "" {
		cfg.OutputName = "toned_beats.json"
	}
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	p := vamp.New("worldsmith-beat-toner").
		Describe("Rewrite a chapter's beats so its sealed material stays subtext (same events).")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the assembled author's notebook (the sealed material to keep off-page)."))
	p.Input("chapter_n", vamp.WithDefault(fmt.Sprintf("%d", cfg.ChapterN)),
		vamp.Describe("This chapter's number."))
	p.Input("title", vamp.WithDefault(cfg.Title),
		vamp.Describe("This chapter's title."))
	p.Input("beats", vamp.WithDefault(cfg.BeatsRendered),
		vamp.Describe("The chapter's current beats."))
	p.Input("constraints", vamp.WithDefault(fallbackStr(cfg.ConstraintsRendered, "(none)")),
		vamp.Describe("The chapter's current constraints."))
	p.Input("reveals", vamp.WithDefault(fallbackStr(cfg.Reveals, "(this chapter reveals nothing — keep ALL sealed material subtext)")),
		vamp.Describe("This chapter's reveal-license: what MAY be shown; everything else stays sealed."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during beat toning")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	p.Text("beat_toner").
		Capability("long_form").
		PromptFS(PromptsFS, "beat_toner.md").
		OutputFormatJSON().
		Output(cfg.OutputName).
		Param("temperature", 0.3).
		Param("max_tokens", 6144).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	return p.Build()
}
