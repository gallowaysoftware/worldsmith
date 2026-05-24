// Command worldsmith turns human worldbuilding into LLM-extrapolated
// fiction rendered as audiobook m4bs.
//
//	worldsmith init <slug>           Scaffold a new world dir.
//	worldsmith story <slug>          Generate the next installment.
//	worldsmith list                  Show all worlds + installment counts.
//	worldsmith activate              Bring up every required vibe profile.
//	worldsmith doctor                Read-only: what's running, what's missing.
//
// All flow is opt-in: the user authors world.md / characters.json /
// brief.md, then `worldsmith story <slug>` runs the pipeline. Each
// finished installment writes summary.md + canon_delta.md alongside
// the m4b; the next call reads those before drafting so continuity
// builds without the user re-feeding context.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/vibe/vamp"

	"github.com/gallowaysoftware/worldsmith/internal/pipeline"
	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// Package-level vars Cobra binds for each subcommand, before the
// vamp factory closure fires. Each subcommand reads only its own.
var (
	storySlug         string
	storyInstallment  int
	storyNarrator     string
	storyPublishTo    string
)

func main() {
	root := &cobra.Command{
		Use:   "worldsmith",
		Short: "Human worldbuilding → LLM-extrapolated fiction → audiobook.",
		Long: `worldsmith spins fictional installments out of a world bible you
author. You write world.md (setting, history, factions, tone) and
characters.json; the LLM writes the prose. Each installment carries
forward via an auto-grown canon doc + per-installment briefs you
write between calls.

Bring your own vibe daemon + long_form / tts_kokoro / comfyui
profiles (worldsmith activate brings them all up).`,
		SilenceUsage: true,
	}

	root.AddCommand(initCommand())
	root.AddCommand(storyCommand())
	root.AddCommand(listCommand())
	root.AddCommand(activateCommand())
	root.AddCommand(doctorCommand())
	root.AddCommand(timelineCommand())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "worldsmith:", err)
		os.Exit(1)
	}
}

func initCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init <slug>",
		Short: "Scaffold a new world: world.md + characters.json + briefs/001.md stubs.",
		Long: `init drops a starter world layout under
~/.local/state/worldsmith/<slug>/ with the three files you'll
edit before the first story run. Idempotent — re-running on an
existing slug doesn't clobber your edits, only fills missing
files.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			if !isSlug(slug) {
				return fmt.Errorf("slug %q must be lowercase letters / digits / hyphens", slug)
			}
			layout, err := world.Open(slug)
			if err != nil {
				return err
			}
			if err := world.ScaffoldWorld(layout, slug); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ world scaffold dropped at %s\n", layout.Root)
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintln(cmd.OutOrStdout(), "edit these before the first story:")
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", layout.WorldFile())
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", layout.CharactersFile())
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", layout.BriefFile(1))
			fmt.Fprintln(cmd.OutOrStdout(), "")
			fmt.Fprintf(cmd.OutOrStdout(), "then: worldsmith activate && worldsmith story %s\n", slug)
			return nil
		},
	}
}

func storyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "story <slug>",
		Short: "Generate the next installment of a world.",
		Long: `story picks the smallest 1-indexed installment number whose
episode.m4b doesn't yet exist, reads briefs/NNN.md for that
number's direction, assembles all prior summaries + canon into
context, and runs the per-installment pipeline.

The pipeline:
  write_story   draft prose (5-8k words)
  edit_story    quality / cut pass
  canon_delta   atomic facts → canon.md
  summarize     200-400 word recap → priors_file for next call
  compose_cover SDXL prompt for installment cover
  generate_cover ComfyUI runs the SDXL workflow
  showrunner    paragraphs → narration script
  cast_voice    Kokoro per-segment TTS
  mix_episode   concat + loudnorm → episode.m4b (with cover + metadata)

--installment N regenerates a specific installment instead of the
next pending. Useful for iterating on a draft.

--publish-to <dir> copies the finished episode.m4b into a podcast /
audiobook library with the audiobookshelf-friendly name "NNN -
Installment NNN.m4b".`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := storySlug
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required (positional arg or --slug)")
			}
			return runStory(cmd, slug)
		},
	}
	cmd.Flags().StringVar(&storySlug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().IntVar(&storyInstallment, "installment", 0, "Specific installment number to (re)generate. 0 = next pending.")
	cmd.Flags().StringVar(&storyNarrator, "narrator", "am_fenrir", "Kokoro voice id for the narrator.")
	cmd.Flags().StringVar(&storyPublishTo, "publish-to", "", "Directory to copy the finished episode.m4b into.")
	return cmd
}

func runStory(cmd *cobra.Command, slug string) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}

	// Sanity check: world.md must exist + be non-stub-ish (the stub
	// has a comment line we can detect). We don't enforce this hard,
	// just warn — the user might genuinely intend a minimal world.
	if _, err := os.Stat(layout.WorldFile()); err != nil {
		return fmt.Errorf("world.md not found at %s — run `worldsmith init %s` first", layout.WorldFile(), slug)
	}

	n := storyInstallment
	if n == 0 {
		n, err = world.NextInstallment(layout)
		if err != nil {
			return err
		}
	}

	// Brief required — this is the per-installment human direction.
	if _, err := os.Stat(layout.BriefFile(n)); err != nil {
		return fmt.Errorf("brief not found at %s — write one before running story %d", layout.BriefFile(n), n)
	}

	canonPath, err := world.EnsureCanonFile(layout)
	if err != nil {
		return fmt.Errorf("ensure canon: %w", err)
	}

	installmentDir := layout.InstallmentDir(n)
	if err := os.MkdirAll(installmentDir, 0o755); err != nil {
		return err
	}
	priorsPath, err := world.EnsurePriorsFile(layout, installmentDir, n)
	if err != nil {
		return fmt.Errorf("ensure priors: %w", err)
	}

	// Pre-pipeline: parse the brief's YAML frontmatter (if any) for
	// year_override / pov_region / on_stage_actors, then compute the
	// filtered timeline view and write it to historical_context.md
	// in the run dir. The writer prompt reads that file verbatim.
	brief, _, err := world.ParseBrief(layout.BriefFile(n))
	if err != nil {
		return fmt.Errorf("parse brief frontmatter: %w", err)
	}
	timeline, err := world.LoadTimeline(layout)
	if err != nil {
		return fmt.Errorf("load timeline: %w", err)
	}
	filterOpts := world.FilterOptsFromBrief(brief, timeline.Calendar)
	historyPath, err := world.WriteHistoricalContext(installmentDir, timeline.Events, filterOpts)
	if err != nil {
		return fmt.Errorf("write historical context: %w", err)
	}

	cfg := pipeline.StoryConfig{
		InstallmentNumber:     n,
		WorldFile:             layout.WorldFile(),
		CharactersFile:        layout.CharactersFile(),
		CanonFile:             canonPath,
		PriorsFile:            priorsPath,
		BriefFile:             layout.BriefFile(n),
		HistoricalContextFile: historyPath,
		NarratorVoice:         storyNarrator,
	}

	fmt.Fprintf(cmd.OutOrStdout(), "world: %s\n", slug)
	fmt.Fprintf(cmd.OutOrStdout(), "installment: %d\n", n)
	fmt.Fprintf(cmd.OutOrStdout(), "brief: %s\n", layout.BriefFile(n))
	fmt.Fprintln(cmd.OutOrStdout(), "")

	root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
		return pipeline.BuildStory(cfg)
	})
	if err != nil {
		return err
	}
	root.SetArgs([]string{"run", "--run-dir", installmentDir})
	if err := root.Execute(); err != nil {
		return fmt.Errorf("installment %d: %w", n, err)
	}

	// Post-run: fold this installment's canon_delta into the
	// running canon.md so the next call reads it.
	if err := world.AppendCanonDelta(layout, n); err != nil {
		return fmt.Errorf("append canon: %w", err)
	}

	localM4B := layout.InstallmentFile(n, "episode.m4b")
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ installment %d done: %s\n", n, localM4B)

	if storyPublishTo != "" {
		name := fmt.Sprintf("%03d - Installment %d.m4b", n, n)
		dst := filepath.Join(storyPublishTo, name)
		if err := os.MkdirAll(storyPublishTo, 0o755); err != nil {
			return fmt.Errorf("mkdir publish dir: %w", err)
		}
		if err := copyFile(localM4B, dst); err != nil {
			return fmt.Errorf("publish to %s: %w", dst, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  published: %s\n", dst)
	}
	return nil
}

// stubPipelineFactory returns a factory that builds the story
// pipeline with empty-but-valid input paths. activate/doctor only
// read RequireProfile + RequireService declarations, not the inputs,
// so this is enough to satisfy vamp's pre-flight without any
// real world on disk.
func stubPipelineFactory() func() (*vamp.Pipeline, error) {
	return func() (*vamp.Pipeline, error) {
		return pipeline.BuildStory(pipeline.StoryConfig{
			InstallmentNumber:     1,
			WorldFile:             os.DevNull,
			CharactersFile:        os.DevNull,
			CanonFile:             os.DevNull,
			PriorsFile:            os.DevNull,
			BriefFile:             os.DevNull,
			HistoricalContextFile: os.DevNull,
			NarratorVoice:         "am_fenrir",
		})
	}
}

func activateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "activate",
		Short: "Bring up every vibe profile / service worldsmith needs.",
		Long: `activate starts the long_form LLM profile + tts_kokoro and
comfyui services so worldsmith story can run. Idempotent — already-
running services are left alone. Same plumbing as vibe's per-
pipeline activate; delegates through vamp.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := vamp.BuildRoot(stubPipelineFactory())
			if err != nil {
				return err
			}
			root.SetArgs([]string{"activate"})
			return root.Execute()
		},
	}
}

func doctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Read-only: report which required vibe services are up.",
		Long: `doctor probes each declared profile + service URL and prints a
status line per requirement. Exits non-zero if anything is missing
so it works as a CI gate. Doesn't start anything itself.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := vamp.BuildRoot(stubPipelineFactory())
			if err != nil {
				return err
			}
			root.SetArgs([]string{"doctor"})
			return root.Execute()
		},
	}
}

func listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List worlds + installment counts.",
		RunE: func(cmd *cobra.Command, args []string) error {
			slugs, err := world.List()
			if err != nil {
				return err
			}
			if len(slugs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no worlds yet — `worldsmith init <slug>` to start one")
				return nil
			}
			sort.Strings(slugs)
			for _, slug := range slugs {
				layout, err := world.Open(slug)
				if err != nil {
					continue
				}
				done, _ := world.CompletedInstallments(layout)
				fmt.Fprintf(cmd.OutOrStdout(), "  %-32s  %d installment(s)\n", slug, len(done))
			}
			return nil
		},
	}
}

// isSlug enforces the same shape vibe profiles use: lowercase
// alphanumerics + hyphens. Keeps the layout-on-disk safe from
// adversarial / accidentally weird slugs.
func isSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}

// copyFile streams src → dst with the system default buffer.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

