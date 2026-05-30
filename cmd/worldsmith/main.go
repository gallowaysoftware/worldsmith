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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/vibe/vamp"

	"github.com/gallowaysoftware/worldsmith/internal/pipeline"
	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// Package-level vars Cobra binds for each subcommand, before the
// vamp factory closure fires. Each subcommand reads only its own.
var (
	storySlug        string
	storyInstallment int
	storyNarrator    string
	storyPublishTo   string
	storyCandidates  int
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
	root.AddCommand(novelCommand())
	root.AddCommand(listCommand())
	root.AddCommand(activateCommand())
	root.AddCommand(doctorCommand())
	root.AddCommand(timelineCommand())
	root.AddCommand(benchCommand())

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
	cmd.Flags().IntVar(&storyCandidates, "candidates", 1, "Generate N outline candidates, score them, and write from the best. 1 = no rerank.")
	return cmd
}

func novelCommand() *cobra.Command {
	var (
		slug           string
		targetChapters int
		narrator       string
		publishTo      string
		candidates     int
	)
	cmd := &cobra.Command{
		Use:   "novel <slug>",
		Short: "Generate a multi-chapter novel from arc.json.",
		Long: `novel drives a long-arc work from arc.json — a title, a premise,
and an ordered list of chapter beats. Each beat becomes a per-chapter
brief and runs the same installment pipeline as ` + "`story`" + `, in
sequence, so canon, prior summaries, continuity checks, and prose
metrics roll forward chapter to chapter.

If arc.json doesn't exist yet, novel writes a stub and stops so you
can fill in the chapters. Finished chapters are skipped on re-run, so
an interrupted novel resumes where it left off.

--target-chapters caps how many of arc.json's chapters to generate
this run (0 = all). When ffmpeg is available the finished chapters are
concatenated into a single book.m4b.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required (positional arg or --slug)")
			}
			// generateInstallment reads the package-level candidate
			// count; honour novel's own flag.
			storyCandidates = candidates
			return runNovel(cmd, slug, targetChapters, narrator, publishTo)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().IntVar(&targetChapters, "target-chapters", 0, "Max chapters to generate this run (0 = all in arc.json).")
	cmd.Flags().StringVar(&narrator, "narrator", "am_fenrir", "Kokoro voice id for the narrator.")
	cmd.Flags().StringVar(&publishTo, "publish-to", "", "Directory to copy the finished book.m4b into.")
	cmd.Flags().IntVar(&candidates, "candidates", 1, "Per-chapter outline candidates to score and pick from. 1 = no rerank.")
	return cmd
}

func runNovel(cmd *cobra.Command, slug string, targetChapters int, narrator, publishTo string) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	if _, err := os.Stat(layout.WorldFile()); err != nil {
		return fmt.Errorf("world.md not found at %s — run `worldsmith init %s` first", layout.WorldFile(), slug)
	}

	arc, ok, err := world.LoadArc(layout)
	if err != nil {
		return err
	}
	if !ok {
		if err := world.ScaffoldArc(layout); err != nil {
			return err
		}
		return fmt.Errorf("no arc.json yet — wrote a stub at %s; fill in the chapter beats, then re-run `worldsmith novel %s`", layout.ArcFile(), slug)
	}
	if len(arc.Chapters) == 0 {
		return fmt.Errorf("arc.json at %s has no chapters — add entries under \"chapters\"", layout.ArcFile())
	}

	count := len(arc.Chapters)
	if targetChapters > 0 && targetChapters < count {
		count = targetChapters
	}
	if targetChapters > len(arc.Chapters) {
		fmt.Fprintf(cmd.ErrOrStderr(), "note: arc.json has %d chapters; generating all of them (--target-chapters %d ignored)\n",
			len(arc.Chapters), targetChapters)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "world: %s\n", slug)
	fmt.Fprintf(cmd.OutOrStdout(), "novel: %s (%d chapters)\n\n", fallbackStr(arc.Title, slug), count)

	for i := 1; i <= count; i++ {
		// Resume: a finished chapter already has its episode.m4b.
		if _, err := os.Stat(layout.InstallmentFile(i, "episode.m4b")); err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "chapter %d: already done — skipping\n", i)
			continue
		}
		// Materialise the chapter's brief from its arc beat if the user
		// hasn't hand-written one. A hand-written brief always wins.
		briefPath := layout.BriefFile(i)
		if _, err := os.Stat(briefPath); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			if err := os.MkdirAll(layout.BriefsDir(), 0o755); err != nil {
				return err
			}
			content := world.RenderBriefFromBeat(i, arc.Chapters[i-1])
			if err := os.WriteFile(briefPath, []byte(content), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "chapter %d: wrote brief from arc beat\n", i)
		}
		if err := generateInstallment(cmd, layout, i, narrator); err != nil {
			return fmt.Errorf("chapter %d: %w", i, err)
		}
	}

	// Stitch the chapter m4bs into one book file (best-effort).
	bookPath, err := assembleBook(cmd, layout, count)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "note: %v\n", err)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ novel assembled: %s\n", bookPath)

	if publishTo != "" {
		name := sanitizeFilenameFragment(fallbackStr(arc.Title, slug))
		if name == "" {
			name = "novel"
		}
		dst := filepath.Join(publishTo, name+".m4b")
		if err := os.MkdirAll(publishTo, 0o755); err != nil {
			return fmt.Errorf("mkdir publish dir: %w", err)
		}
		if err := copyFile(bookPath, dst); err != nil {
			return fmt.Errorf("publish to %s: %w", dst, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  published: %s\n", dst)
	}
	return nil
}

// assembleBook concatenates the per-chapter episode.m4b files into a
// single book.m4b under the world root, using ffmpeg's concat demuxer
// with stream copy (no re-encode — every chapter came off the same
// pipeline, so codecs match). Returns a descriptive error (not a hard
// failure) when ffmpeg is absent so the per-chapter m4bs still stand
// on their own.
func assembleBook(cmd *cobra.Command, layout world.Layout, count int) (string, error) {
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found; per-chapter m4bs are under %s", layout.InstallmentsDir())
	}
	var paths []string
	for i := 1; i <= count; i++ {
		p := layout.InstallmentFile(i, "episode.m4b")
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("no chapter m4bs found to assemble")
	}
	listPath := filepath.Join(layout.Root, "book_concat.txt")
	var b strings.Builder
	for _, p := range paths {
		// ffmpeg concat list: single-quote the path, escaping any
		// embedded quote as '\''.
		fmt.Fprintf(&b, "file '%s'\n", strings.ReplaceAll(p, "'", `'\''`))
	}
	if err := os.WriteFile(listPath, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	bookPath := filepath.Join(layout.Root, "book.m4b")
	c := exec.Command(ff, "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", bookPath)
	c.Stdout = cmd.ErrOrStderr()
	c.Stderr = cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg concat: %w", err)
	}
	return bookPath, nil
}

// fallbackStr returns s trimmed, or d when s is blank.
func fallbackStr(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return strings.TrimSpace(s)
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

	fmt.Fprintf(cmd.OutOrStdout(), "world: %s\n", slug)
	if err := generateInstallment(cmd, layout, n, storyNarrator); err != nil {
		return err
	}

	if storyPublishTo != "" {
		if err := publishEpisode(cmd, layout, n, storyPublishTo); err != nil {
			return err
		}
	}
	return nil
}

// generateInstallment runs the full per-installment pipeline (prose →
// canon/summary/continuity → cover → narration → audiobook) for one
// numbered installment whose brief already exists on disk. It is the
// shared core behind both `story` (one installment) and `novel` (a
// sequence of chapters), so canon, priors, retrieval, continuity, and
// metrics behave identically whichever entry point drove it.
func generateInstallment(cmd *cobra.Command, layout world.Layout, n int, narrator string) error {
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
	brief, briefBody, err := world.ParseBrief(layout.BriefFile(n))
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

	// Relevance-filter the running canon down to what this brief needs
	// (world rules + on-stage-actor facts always kept). For a small
	// canon this is a verbatim copy; it only trims once the document
	// outgrows the budget. The outline + writer read this; canon_delta
	// + continuity still read the full canon.
	canonRelevantPath, err := world.WriteRelevantCanon(
		installmentDir, canonPath, briefBody, brief.OnStageActors, world.DefaultCanonBudget)
	if err != nil {
		return fmt.Errorf("filter canon: %w", err)
	}

	cfg := pipeline.StoryConfig{
		InstallmentNumber:     n,
		WorldFile:             layout.WorldFile(),
		CharactersFile:        layout.CharactersFile(),
		CanonFile:             canonPath,
		CanonRelevantFile:     canonRelevantPath,
		PriorsFile:            priorsPath,
		BriefFile:             layout.BriefFile(n),
		HistoricalContextFile: historyPath,
		NarratorVoice:         narrator,
	}

	// Optional candidate rerank: generate several outlines, judge them,
	// and write the winner so the prose stage builds on the strongest
	// plan instead of the first one sampled. No-op when --candidates
	// is 1 (the default).
	if storyCandidates > 1 {
		chosen, err := selectBestOutline(cmd, cfg, installmentDir, storyCandidates)
		if err != nil {
			return fmt.Errorf("candidate outline rerank: %w", err)
		}
		cfg.OutlineJSON = chosen
	}

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

	// Deterministic prose-health read: slop density, the not-X-but-Y
	// reflex, anaphora, repeated trigrams. Written to metrics.json
	// beside the m4b. Non-fatal — a clean run shouldn't break because
	// the audit couldn't run.
	if m, err := world.WriteProseMetrics(
		layout.InstallmentFile(n, "story.md"),
		layout.InstallmentFile(n, "metrics.json"),
	); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(),
			"prose: %d words, slop %.1f/1k (%d hits), not-x-but-y %d\n",
			m.Words, m.SlopPer1000, m.SlopTotal, m.NotXButY)
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: prose metrics: %v\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ installment %d done: %s\n", n, layout.InstallmentFile(n, "episode.m4b"))
	return nil
}

// publishEpisode copies a finished installment's episode.m4b into a
// watch directory with an Audiobookshelf-friendly name derived from
// the brief's H1 title.
func publishEpisode(cmd *cobra.Command, layout world.Layout, n int, publishTo string) error {
	localM4B := layout.InstallmentFile(n, "episode.m4b")
	// Use the brief's H1 title for the filename when present —
	// "001 - The First Hour.m4b" reads better in Audiobookshelf's
	// podcast UI than the generic "001 - Installment 1.m4b". Falls
	// back to the numeric default when the brief has no H1.
	var name string
	if title := sanitizeFilenameFragment(world.BriefTitle(layout.BriefFile(n))); title != "" {
		name = fmt.Sprintf("%03d - %s.m4b", n, title)
	} else {
		name = fmt.Sprintf("%03d - Installment %d.m4b", n, n)
	}
	dst := filepath.Join(publishTo, name)
	if err := os.MkdirAll(publishTo, 0o755); err != nil {
		return fmt.Errorf("mkdir publish dir: %w", err)
	}
	if err := copyFile(localM4B, dst); err != nil {
		return fmt.Errorf("publish to %s: %w", dst, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  published: %s\n", dst)
	return nil
}

// selectBestOutline generates n outline candidates (the same outline
// stage the full pipeline uses, sampled at spread temperatures so the
// plans genuinely differ), scores each structurally, and returns the
// JSON of the highest-scoring one. The winner is fed to the story
// pipeline via StoryConfig.OutlineJSON. Re3's generate-N-and-rerank,
// applied at the plan level where it's cheapest and highest-leverage.
func selectBestOutline(cmd *cobra.Command, cfg pipeline.StoryConfig, installmentDir string, n int) (string, error) {
	candRoot := filepath.Join(installmentDir, "outline_candidates")
	if err := os.MkdirAll(candRoot, 0o755); err != nil {
		return "", err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "outline rerank: %d candidates\n", n)

	var bestJSON string
	var bestScore world.OutlineScore
	bestSet := false
	for i := 0; i < n; i++ {
		sub := filepath.Join(candRoot, fmt.Sprintf("%d", i))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			return "", err
		}
		temp := 0.3 + 0.15*float64(i) // 0.30, 0.45, 0.60, ...
		ocfg := pipeline.OutlineConfig{
			WorldFile:             cfg.WorldFile,
			CharactersFile:        cfg.CharactersFile,
			CanonFile:             cfg.CanonFile,
			CanonRelevantFile:     cfg.CanonRelevantFile,
			PriorsFile:            cfg.PriorsFile,
			BriefFile:             cfg.BriefFile,
			HistoricalContextFile: cfg.HistoricalContextFile,
			Temperature:           temp,
		}
		root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
			return pipeline.BuildOutline(ocfg)
		})
		if err != nil {
			return "", err
		}
		root.SetArgs([]string{"run", "--run-dir", sub, "--no-cache"})
		if err := root.Execute(); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  candidate %d failed: %v\n", i, err)
			continue
		}
		raw, err := os.ReadFile(filepath.Join(sub, "outline.json"))
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "  candidate %d: no outline.json\n", i)
			continue
		}
		score := world.ScoreOutline(string(raw))
		fmt.Fprintf(cmd.OutOrStdout(), "  candidate %d (temp %.2f): score %.1f\n", i, temp, score.Total)
		if !bestSet || score.Total > bestScore.Total {
			bestJSON, bestScore, bestSet = string(raw), score, true
		}
	}
	if !bestSet {
		return "", fmt.Errorf("all %d outline candidates failed", n)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  chosen: score %.1f\n", bestScore.Total)
	return bestJSON, nil
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

// sanitizeFilenameFragment strips characters that don't belong in a
// filename when the path may be exposed over SMB to Windows clients
// (Audiobookshelf libraries often live on NAS shares). Removes the
// Windows reserved set `<>:"/\|?*` plus NUL and control chars,
// collapses runs of whitespace into single spaces, and trims
// leading/trailing dots and spaces (which Windows Explorer mangles).
// Returns the empty string if the result would be empty.
func sanitizeFilenameFragment(s string) string {
	const forbidden = "<>:\"/\\|?*"
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		// Whitespace (incl. tab) collapses to single space.
		if r == ' ' || r == '\t' {
			if prevSpace {
				continue
			}
			b.WriteRune(' ')
			prevSpace = true
			continue
		}
		// Strip remaining control chars (NUL, BEL, etc.) + DEL.
		if r < 0x20 || r == 0x7f {
			continue
		}
		if strings.ContainsRune(forbidden, r) {
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.Trim(b.String(), " .")
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
