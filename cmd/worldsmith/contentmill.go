package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/vibe/vamp"

	"github.com/gallowaysoftware/worldsmith/internal/pipeline"
	"github.com/gallowaysoftware/worldsmith/internal/world"
)

var (
	worldgenTheme string
	worldgenCount int
	worldgenSlug  string

	sceneSlug      string
	sceneShots     int
	sceneNarrator  string
	scenePublishTo string
	sceneFormat    string
)

// worldgenCommand auto-authors worlds from a theme (content-mill breadth).
func worldgenCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "worldgen --theme <theme>",
		Short: "Auto-author one or more worlds from a creative theme.",
		Long: `worldgen turns a theme into a complete worldsmith world (world.md +
characters.json) using the LLM, ready for ` + "`worldsmith scene`" + `. Run with
--count N to generate several distinct worlds in one go. The result is a
standard world dir, so every other worldsmith command works on it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if worldgenTheme == "" {
				return fmt.Errorf("--theme is required")
			}
			made := 0
			for i := 0; i < worldgenCount; i++ {
				slug, err := generateOneWorld(cmd, worldgenTheme)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "worldgen %d/%d failed: %v\n", i+1, worldgenCount, err)
					continue
				}
				made++
				fmt.Fprintf(cmd.OutOrStdout(), "✓ world %d/%d: %s\n", i+1, worldgenCount, slug)
			}
			if made == 0 {
				return fmt.Errorf("no worlds generated")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&worldgenTheme, "theme", "", "Creative theme / niche for the world(s).")
	cmd.Flags().IntVar(&worldgenCount, "count", 1, "Number of distinct worlds to generate.")
	cmd.Flags().StringVar(&worldgenSlug, "slug", "", "Force a slug (single-world only; default derived from the world name).")
	return cmd
}

// generateOneWorld runs the worldgen pipeline (with a couple of retries on a
// thin result) and persists the world. Returns the slug written.
func generateOneWorld(cmd *cobra.Command, theme string) (string, error) {
	genDir, err := os.MkdirTemp(world.DefaultRoot(), ".worldgen-")
	if err != nil {
		if mkErr := os.MkdirAll(world.DefaultRoot(), 0o755); mkErr != nil {
			return "", mkErr
		}
		genDir, err = os.MkdirTemp(world.DefaultRoot(), ".worldgen-")
		if err != nil {
			return "", err
		}
	}
	defer os.RemoveAll(genDir)

	var seed world.WorldSeed
	var raw []byte
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
			return pipeline.BuildWorldGen(pipeline.WorldGenConfig{Theme: theme})
		})
		if err != nil {
			return "", err
		}
		root.SetArgs([]string{"run", "--run-dir", genDir, "--no-cache"})
		if err := root.Execute(); err != nil {
			lastErr = err
			continue
		}
		raw, err = os.ReadFile(filepath.Join(genDir, "world.json"))
		if err != nil {
			lastErr = err
			continue
		}
		seed, err = world.ParseWorldSeed(raw)
		if err != nil {
			lastErr = err
			continue
		}
		if err := seed.Validate(); err != nil {
			lastErr = err
			fmt.Fprintf(cmd.ErrOrStderr(), "  attempt %d: thin world (%v), regenerating...\n", attempt, err)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return "", lastErr
	}

	slug := worldgenSlug
	if slug == "" || worldgenCount > 1 {
		slug = uniqueWorldSlug(seed.Slug())
	}
	layout, err := world.Open(slug)
	if err != nil {
		return "", err
	}
	if err := world.WriteWorldFromSeed(layout, seed, raw); err != nil {
		return "", err
	}
	return slug, nil
}

// uniqueWorldSlug appends -2, -3, ... if a world dir already exists, so a
// --count run never overwrites a freshly-made world.
func uniqueWorldSlug(base string) string {
	if _, err := os.Stat(filepath.Join(world.DefaultRoot(), base)); err != nil {
		return base
	}
	for i := 2; i < 1000; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if _, err := os.Stat(filepath.Join(world.DefaultRoot(), cand)); err != nil {
			return cand
		}
	}
	return base
}

// sceneCommand turns a world into one vertical short-form video.
func sceneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scene <slug>",
		Short: "Generate the next vertical video scene for a world.",
		Long: `scene invents a self-contained ~30s scene set in the world, then for each
shot generates a still (Qwen-Image), animates it (Wan2.2 image-to-video),
voices the narration (Kokoro), and assembles a vertical 1080x1920 MP4 with
burned captions.

Runs in two phases so the LLM is unloaded before the image model loads:
phase 1 (LLM) writes the shot list; phase 2 (ComfyUI + TTS) renders it.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := sceneSlug
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required (positional arg or --slug)")
			}
			return runScene(cmd, slug)
		},
	}
	cmd.Flags().StringVar(&sceneSlug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().IntVar(&sceneShots, "shots", 7, "Number of shots in the scene.")
	cmd.Flags().StringVar(&sceneNarrator, "narrator", "am_fenrir", "Default Kokoro voice for the narrator.")
	cmd.Flags().StringVar(&scenePublishTo, "publish-to", "", "Directory to copy the finished final.mp4 into.")
	cmd.Flags().StringVar(&sceneFormat, "format", "", "TikTok format brief (default: rotates by scene number).")
	return cmd
}

func runScene(cmd *cobra.Command, slug string) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	if _, err := os.Stat(layout.WorldFile()); err != nil {
		return fmt.Errorf("world.md not found at %s — run `worldsmith worldgen` or `worldsmith init %s` first", layout.WorldFile(), slug)
	}
	canonPath, err := world.EnsureCanonFile(layout)
	if err != nil {
		return err
	}

	n, err := world.NextScene(layout)
	if err != nil {
		return err
	}
	sceneDir := layout.SceneDir(n)
	if err := os.MkdirAll(sceneDir, 0o755); err != nil {
		return err
	}

	format := sceneFormat
	if format == "" {
		format = pipeline.FormatForScene(n)
	}
	cfg := pipeline.SceneConfig{
		WorldFile:      layout.WorldFile(),
		CharactersFile: layout.CharactersFile(),
		CanonFile:      canonPath,
		Shots:          sceneShots,
		NarratorVoice:  sceneNarrator,
		Format:         format,
		ShotsFile:      filepath.Join(sceneDir, "shots.json"),
	}

	fmt.Fprintf(cmd.OutOrStdout(), "world: %s\nscene: %03d\nformat: %s\n", slug, n, format)

	// Phase 1: LLM writes the shot list.
	fmt.Fprintln(cmd.OutOrStdout(), "phase 1/2: writing scene shot list (LLM)...")
	scriptRoot, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
		return pipeline.BuildSceneScript(cfg)
	})
	if err != nil {
		return err
	}
	scriptRoot.SetArgs([]string{"run", "--run-dir", sceneDir, "--no-cache"})
	if err := scriptRoot.Execute(); err != nil {
		return fmt.Errorf("scene %d phase 1: %w", n, err)
	}

	// Defensive: normalize every shot's voice_id to a known Kokoro voice so a
	// hallucinated/typo'd voice from the outline LLM can't 400 the TTS stage
	// and fail the whole render. (worldgen already normalizes characters.json,
	// but the outline can introduce its own.)
	if err := normalizeShotVoices(cfg.ShotsFile); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "  note: normalize shot voices: %v\n", err)
	}

	// Free the LLM's VRAM before the image model loads — they can't co-reside
	// on a 32GB card. Best-effort: a failure here just means the daemon's
	// VRAM preflight will reject the image stage with a clear message.
	freeActiveProfile(cmd)

	// Phase 2: ComfyUI + TTS render the shots into a vertical short.
	fmt.Fprintln(cmd.OutOrStdout(), "phase 2/2: rendering shots (image -> video -> voice -> assemble)...")
	renderRoot, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
		return pipeline.BuildSceneRender(cfg)
	})
	if err != nil {
		return err
	}
	renderRoot.SetArgs([]string{"run", "--run-dir", sceneDir, "--no-cache"})
	if err := renderRoot.Execute(); err != nil {
		return fmt.Errorf("scene %d phase 2: %w", n, err)
	}

	final := layout.SceneFile(n, "final.mp4")
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ scene %03d: %s\n", n, final)

	if scenePublishTo != "" {
		if err := os.MkdirAll(scenePublishTo, 0o755); err != nil {
			return err
		}
		dst := filepath.Join(scenePublishTo, fmt.Sprintf("%s-%03d.mp4", slug, n))
		if err := copyFile(final, dst); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  published: %s\n", dst)
	}
	return nil
}

// normalizeShotVoices rewrites shots.json in place, replacing any shot's
// voice_id that isn't a known Kokoro voice with the narrator fallback.
func normalizeShotVoices(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	changed := false
	for _, it := range doc.Items {
		if v, ok := it["voice_id"].(string); ok {
			if nv := world.NormalizeVoice(v); nv != v {
				it["voice_id"] = nv
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// freeActiveProfile shells `vibe stop` to unload the active LLM profile so the
// image/video models have the GPU to themselves. Non-fatal.
func freeActiveProfile(cmd *cobra.Command) {
	c := exec.Command("vibe", "stop")
	out, err := c.CombinedOutput()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "  note: `vibe stop` to free LLM VRAM failed (%v); continuing\n", err)
		return
	}
	_ = out
}
