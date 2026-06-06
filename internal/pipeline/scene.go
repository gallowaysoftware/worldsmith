package pipeline

import (
	"fmt"
	"time"

	"github.com/gallowaysoftware/vibe/contentkit"
	"github.com/gallowaysoftware/vibe/vamp"
)

// SceneConfig drives `worldsmith scene`: turn a world into one vertical
// short-form video. Run in two pipeline phases (BuildSceneScript then
// BuildSceneRender) so the ~26GB LLM is unloaded before the ~20GB image model
// loads — the two can't co-reside on a 32GB card.
type SceneConfig struct {
	WorldFile      string
	CharactersFile string
	CanonFile      string
	Shots          int
	NarratorVoice  string
	KokoroURL      string
	// Format is the TikTok format brief for this scene. The CLI rotates it per
	// scene (or honors --format) so depth doesn't emit the same shape twice.
	Format string
	// ShotsFile is the path to the phase-1 shots.json, read by phase 2.
	ShotsFile string
}

const defaultKokoroURL = "http://127.0.0.1:8880"

// SceneFormats is the rotation of short-form video formats `scene` cycles
// through so successive videos are structurally different — a POV, a list, a
// tour, a creature feature — rather than the same narrated scene every time.
// Mechanical variety beats asking the model to "be different" (it ignores that).
var SceneFormats = []string{
	"POV: you wake up as one of this world's characters and the next 25 seconds go very wrong. First person, present tense, a little panicked, building to a hard cut.",
	"Ranked: ways to die in this world, least to most embarrassing. Fast, darkly funny, a brand-new way each shot, escalating to the dumbest one.",
	"An unhinged in-universe tour guide wildly overhypes this world to a tourist who is very obviously about to die. Upbeat, oblivious, escalating dread.",
	"Creature feature: hype the most dangerous thing in this world like a nature-doc narrator slowly losing his mind — whispered awe to full panic.",
	"A leaked in-world voice log / transmission that absolutely was not supposed to get out. Build dread fast, end on one chilling line.",
	"A jaded local rates this world's factions, brutally honest and a little unhinged, worst to best.",
	"Things in this world that would NOT fly on Earth. Listicle energy, escalating to the most unhinged one.",
	"Day in the life of a working-class nobody in this world — mundane, relatable, and then it escalates fast.",
}

// FormatForScene returns the rotated format for a 1-indexed scene number.
func FormatForScene(n int) string {
	if n < 1 {
		n = 1
	}
	return SceneFormats[(n-1)%len(SceneFormats)]
}

// BuildSceneScript is phase 1 (LLM only): invent a scene and break it into
// shots. Writes shots.json (a {"items":[...]} array, each shot tagged with a
// stable idx) into the run dir for phase 2 to consume.
func BuildSceneScript(cfg SceneConfig) (*vamp.Pipeline, error) {
	if cfg.Shots <= 0 {
		cfg.Shots = 7
	}
	if cfg.Format == "" {
		cfg.Format = SceneFormats[0]
	}
	p := vamp.New("worldsmith-scene-script").
		Describe("Invent a short-form video scene from a world and break it into shots.")

	p.Input("world_file", vamp.Required(), vamp.WithDefault(cfg.WorldFile),
		vamp.Describe("Path to world.md."))
	p.Input("characters_file", vamp.Required(), vamp.WithDefault(cfg.CharactersFile),
		vamp.Describe("Path to characters.json."))
	p.Input("canon_file", vamp.WithDefault(cfg.CanonFile),
		vamp.Describe("Path to canon.md (may be empty)."))
	p.Input("shots", vamp.WithDefault(fmt.Sprintf("%d", cfg.Shots)),
		vamp.Describe("Number of shots in the scene."))
	p.Input("format", vamp.WithDefault(cfg.Format),
		vamp.Describe("The TikTok format brief for this scene."))

	p.RequireProfile("long_form")
	p.RequireGPUMemory("~30GB during generation")
	p.CapabilityModel("long_form", vamp.ModelHint{
		MinParams: "27B", MinContext: 131072,
		SuggestedModel: "qwen3.6-27b-mtp-q6_k",
	})

	outline := p.Text("scene_outline").
		Capability("long_form").
		PromptFS(PromptsFS, "scene_outline.md").
		OutputFormatJSON().
		Output("scene.json").
		Param("temperature", 0.75).
		Param("max_tokens", 8192).
		Param("chat_template_kwargs", map[string]any{"enable_thinking": false}).
		Retry(&vamp.RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: 5 * time.Second,
			MaxBackoff:     30 * time.Second,
			RetryOn:        []string{"transient", "invalid_output"},
		})

	// Deterministic render: tag each shot with a stable idx and re-emit as a
	// {"items":[...]} array the per-shot foreach stages fan out over.
	contentkit.EnumerateItems(p, contentkit.EnumerateConfig{
		From:      outline,
		StageName: "enumerate_shots",
		Output:    "shots.json",
		ArrayKey:  "shots",
		IndexKey:  "idx",
		Fields:    []string{"image_prompt", "motion", "narration", "speaker", "voice_id"},
	})

	return p.Build()
}

// BuildSceneRender is phase 2 (ComfyUI + TTS + assembly, no LLM): for each
// shot generate a still (Qwen-Image), animate it (Wan i2v), voice the
// narration (Kokoro), then assemble a vertical MP4 with burned captions.
func BuildSceneRender(cfg SceneConfig) (*vamp.Pipeline, error) {
	if cfg.NarratorVoice == "" {
		cfg.NarratorVoice = "am_fenrir"
	}
	if cfg.KokoroURL == "" {
		cfg.KokoroURL = defaultKokoroURL
	}
	p := vamp.New("worldsmith-scene-render").
		Describe("Render a scene's shots into a vertical short: image -> i2v -> voice -> assemble.")

	p.Input("shots_file", vamp.Required(), vamp.WithDefault(cfg.ShotsFile),
		vamp.Describe("Path to phase-1 shots.json."))

	p.RequireService("comfyui", "http://127.0.0.1:8188",
		"ComfyUI — Qwen-Image stills + Wan2.2 image-to-video.",
		"vibe start comfyui")
	p.RequireService("kokoro-tts", cfg.KokoroURL,
		"Kokoro-FastAPI TTS — per-character narration.",
		"vibe start tts_kokoro")
	p.RequireGPUMemory("~20GB (Qwen-Image) then ~12GB (Wan i2v); run with the LLM unloaded")
	p.RequireDiskSpace("~100MB per scene (stills + clips + final MP4)")

	// Re-emit the shots array so the foreach stages have a JSON-array source.
	shots := p.Render("load_shots").
		Prompt(`{{ readFile .inputs.shots_file }}`).
		Output("shots_loaded.json").
		OutputFormatJSON()

	// Per-shot still. The image_prompt is self-contained (restates character
	// looks) so the cast stays consistent without a reference-conditioning
	// step. seed varies per shot for compositional variety.
	images := p.ComfyUI("scene_images").
		Capability("image_gen").
		After(shots).
		Foreach(shots, "shot").
		WorkflowFS(WorkflowsFS, "qwen_portrait.json").
		Parameter("4.text", "{{ .shot.image_prompt }}").
		Parameter("7.seed", "{{ .shot.idx }}").
		Output("images/shot_{{ .shot.idx }}.png")

	// Per-shot image-to-video. The still is uploaded into ComfyUI's input
	// dir and bound to the LoadImage node via the input_images feature.
	clips := p.ComfyUI("animate").
		Capability("video_gen").
		After(images).
		Foreach(shots, "shot").
		WorkflowFS(WorkflowsFS, "wan_i2v.json").
		Parameter("4.text", "{{ .shot.motion }}, cinematic, smooth motion").
		Parameter("8.seed", "{{ .shot.idx }}").
		InputImage("6.image", "images/shot_{{ .shot.idx }}.png").
		FreeMemoryAfter().
		Output("clips/shot_{{ .shot.idx }}.mp4")

	// Per-shot voiceover, routed to the speaker's voice.
	voices := p.Audio("voiceover").
		Capability("tts").
		After(shots).
		Foreach(shots, "shot").
		Engine(vamp.AudioEngineKokoro).
		EngineURL(cfg.KokoroURL).
		Voice(fmt.Sprintf(`{{ or .shot.voice_id %q }}`, cfg.NarratorVoice)).
		TextTemplate(`{{ ttsNormalize .shot.narration "" }}`).
		Output("audio/shot_{{ .shot.idx }}.wav")

	// Build the short assembly script: one entry per shot pairing its clip,
	// voiceover, and caption (the narration line).
	assembly := p.Render("assembly_script").
		After(shots).
		Prompt(`{{- $shots := index (parseJSON .stages.load_shots.output) "items" -}}
{"width": 1080, "height": 1920, "fps": 30, "shots": [
{{- range $i, $s := $shots -}}
{{- if $i }},{{ end }}
{"video": "clips/shot_{{ index $s "idx" }}.mp4", "audio": "audio/shot_{{ index $s "idx" }}.wav", "caption": {{ toJSON (index $s "narration") }}}
{{- end }}
]}`).
		Output("assembly.json").
		OutputFormatJSON()

	p.Short("assemble").
		After(clips, voices, assembly).
		ScriptFile("assembly.json").
		Size(1080, 1920).
		FPS(30).
		Output("final.mp4")

	return p.Build()
}
