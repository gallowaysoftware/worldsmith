package pipeline

import (
	"os"

	"github.com/gallowaysoftware/vibe/contentkit"
	"github.com/gallowaysoftware/vibe/vamp"
)

// AskConfig drives "ask the author": a one-shot query answered from the world's
// FULL knowledge — the published bible, established canon, AND the private
// notebook. Unlike content generation, there is no fog of war here: the person
// asking IS the author, so the answer may reveal secrets freely. This is what
// makes the notebook+canon a queryable universe, not just a generation input.
type AskConfig struct {
	WorldFile      string
	CharactersFile string
	CanonFile      string
	NotebookFile   string // assembled notebook; os.DevNull when none
	Question       string
}

// BuildAsk constructs the single-stage pipeline that answers Question from the
// world bible + canon + notebook.
func BuildAsk(cfg AskConfig) (*vamp.Pipeline, error) {
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	p := vamp.New("worldsmith-ask").
		Describe("Answer a question about the world from the author's full knowledge (bible + canon + private notebook).")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the assembled author's notebook (private). os.DevNull when none."))
	p.Input("question", vamp.Required(), vamp.WithDefault(cfg.Question),
		vamp.Describe("The question to answer about the world."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB while answering")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	contentkit.LongFormText(p, "answer", 0.4, 8192).
		PromptFS(PromptsFS, "ask.md").
		Output("answer.md")

	return p.Build()
}
