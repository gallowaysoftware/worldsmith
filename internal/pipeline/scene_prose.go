package pipeline

import (
	"fmt"
	"os"
	"time"

	"github.com/gallowaysoftware/vibe/vamp"
)

// SceneProseConfig drives one scene's generation in the per-scene authoring flow.
// The CLI runs BuildSceneProse once per scene in the outline, sequentially, feeding
// each scene the prose of the scenes already written (PriorProseFile) so voice and
// continuity carry and nothing repeats. Length becomes reliable because each scene is
// written to its own word budget — a target the model honours at scene scale, unlike
// a single "write 10,000 words" pass it does not.
type SceneProseConfig struct {
	WorldFile             string
	CharactersFile        string
	CanonRelevantFile     string
	PriorsFile            string
	BriefFile             string
	HistoricalContextFile string
	NotebookFile          string
	LicensedRevealsFile   string
	// ChapterFactsFile is the per-chapter grounding fact-sheet (exact canon +
	// mechanics this chapter's events touch). Empty/DevNull when none.
	ChapterFactsFile string
	// OutlineFile is the full scene plan (context for THIS scene).
	OutlineFile string
	// SceneSpecFile is this scene's object from the outline (setting/goal/turn/budget).
	SceneSpecFile string
	// PriorProseFile is the concatenation of every earlier scene's prose (empty/DevNull
	// for scene 1).
	PriorProseFile string
	// SceneIndex is 1-based; SceneCount is the total in the installment.
	SceneIndex int
	SceneCount int
	// Seed, when non-zero, is passed as the sampling seed for this scene's write
	// stage. best-of-N varies it per attempt so each attempt samples fresh prose
	// (temperature is 0.8) AND lands a distinct cache key — without a per-attempt
	// seed the content-addressed cache returns byte-identical prose every attempt.
	Seed int
}

// BuildSceneProse constructs the one-stage pipeline that writes a single scene's prose,
// continuing from the prior scenes. Each invocation runs in its own run-dir so the
// shared stage name ("write_scene") doesn't collide in the vamp cache across scenes.
func BuildSceneProse(cfg SceneProseConfig) (*vamp.Pipeline, error) {
	if cfg.NotebookFile == "" {
		cfg.NotebookFile = os.DevNull
	}
	if cfg.LicensedRevealsFile == "" {
		cfg.LicensedRevealsFile = os.DevNull
	}
	if cfg.PriorProseFile == "" {
		cfg.PriorProseFile = os.DevNull
	}
	if cfg.CanonRelevantFile == "" {
		cfg.CanonRelevantFile = os.DevNull
	}
	if cfg.HistoricalContextFile == "" {
		cfg.HistoricalContextFile = os.DevNull
	}
	if cfg.ChapterFactsFile == "" {
		cfg.ChapterFactsFile = os.DevNull
	}

	p := vamp.New("worldsmith-scene-prose").
		Describe("Write one scene of an installment, continuing seamlessly from the prior scenes.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_relevant_file", vamp.WithDefault(cfg.CanonRelevantFile),
		vamp.Describe("Path to the relevance-filtered canon view."))
	p.Input("priors_file", vamp.WithDefault(cfg.PriorsFile),
		vamp.Describe("Path to prior-installment summaries."))
	p.Input("brief_file", vamp.Required(), vamp.WithDefault(cfg.BriefFile),
		vamp.Describe("Path to this installment's brief.md."))
	p.Input("historical_context_file", vamp.WithDefault(cfg.HistoricalContextFile),
		vamp.Describe("Path to the pre-filtered timeline view."))
	p.Input("notebook_file", vamp.WithDefault(cfg.NotebookFile),
		vamp.Describe("Path to the assembled author's notebook (fog of war)."))
	p.Input("licensed_reveals_file", vamp.WithDefault(cfg.LicensedRevealsFile),
		vamp.Describe("Path to the licensed-reveals allow-list."))
	p.Input("chapter_facts_file", vamp.WithDefault(cfg.ChapterFactsFile),
		vamp.Describe("Path to the per-chapter canon fact-sheet (exact mechanics to honour)."))
	p.Input("outline_file", vamp.Required(), vamp.WithDefault(cfg.OutlineFile),
		vamp.Describe("Path to the full scene plan (outline.json)."))
	p.Input("scene_spec_file", vamp.Required(), vamp.WithDefault(cfg.SceneSpecFile),
		vamp.Describe("Path to THIS scene's spec (its object from the outline)."))
	p.Input("prior_prose_file", vamp.WithDefault(cfg.PriorProseFile),
		vamp.Describe("Path to the concatenation of earlier scenes' prose (DevNull for scene 1)."))
	p.Input("scene_index", vamp.WithDefault(fmt.Sprintf("%d", cfg.SceneIndex)),
		vamp.Describe("1-based scene number."))
	p.Input("scene_count", vamp.WithDefault(fmt.Sprintf("%d", cfg.SceneCount)),
		vamp.Describe("Total scenes in the installment."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during scene generation")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	writeScene := p.Text("write_scene").
		Capability("long_form").
		PromptFS(PromptsFS, "write_scene.md").
		Output(fmt.Sprintf("scene_%03d.md", cfg.SceneIndex)).
		Param("temperature", 0.8).
		// 8192 tokens ≈ 6,000 words — ample for any single scene's budget.
		Param("max_tokens", 8192).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Param("repetition_penalty", 1.1).
		Param("presence_penalty", 0.3).
		Param("min_p", 0.05)
	if cfg.Seed != 0 {
		writeScene.Param("seed", cfg.Seed)
	}
	writeScene.
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	return p.Build()
}
