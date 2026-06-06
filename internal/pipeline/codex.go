package pipeline

import (
	"github.com/gallowaysoftware/vibe/contentkit"
	"github.com/gallowaysoftware/vibe/vamp"
)

// CodexConfig drives the companion codex: a reader-facing encyclopedia of the
// world compiled from the PUBLISHED bible + established canon. Deliberately does
// NOT read the private notebook — the codex is what a reader could know, so it
// stays spoiler-safe (fog of war preserved). A browsable proof-of-craft artifact.
type CodexConfig struct {
	WorldFile      string
	CharactersFile string
	CanonFile      string
}

// BuildCodex constructs the single-stage pipeline that compiles codex.md.
func BuildCodex(cfg CodexConfig) (*vamp.Pipeline, error) {
	p := vamp.New("worldsmith-codex").
		Describe("Compile a spoiler-safe companion codex (reader's encyclopedia) from the world bible + canon.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.Required(), vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md (what readers have been shown)."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB while compiling")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	// Low temperature: this is compilation/organisation of established facts, not
	// invention. Large budget for a full encyclopedia.
	contentkit.LongFormText(p, "codex", 0.3, 32768).
		PromptFS(PromptsFS, "codex.md").
		Output("codex.md")

	return p.Build()
}
