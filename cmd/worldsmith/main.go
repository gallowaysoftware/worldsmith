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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

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
	storyBestOf      int = 1
	// regenForce gates the destructive regenerate path: clobbering an
	// already-finished installment's episode.m4b and truncating its canon
	// requires an explicit --force. novel/series set it via their own
	// --force flags before calling into generateInstallment.
	regenForce bool
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
	root.AddCommand(briefCommand())
	root.AddCommand(arcCommand())
	root.AddCommand(seriesCommand())
	root.AddCommand(storyCommand())
	root.AddCommand(novelCommand())
	root.AddCommand(worldgenCommand())
	root.AddCommand(expandCommand())
	root.AddCommand(scoreCommand())
	root.AddCommand(askCommand())
	root.AddCommand(codexCommand())
	root.AddCommand(autopilotCommand())
	root.AddCommand(sceneCommand())
	root.AddCommand(listCommand())
	root.AddCommand(activateCommand())
	root.AddCommand(doctorCommand())
	root.AddCommand(timelineCommand())
	root.AddCommand(benchCommand())

	// Translate Ctrl-C / SIGTERM into a cancelled context that flows through
	// cmd.Context() to every child we launch (vamp runs, ffmpeg/ffprobe, the
	// $EDITOR) via exec.CommandContext — so a signal kills the whole tree
	// instead of orphaning children and leaving half-written artifacts.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := root.ExecuteContext(ctx); err != nil {
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
			unlock, err := lockWorld(layout)
			if err != nil {
				return err
			}
			defer unlock()
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
  outline_story  scene plan + per-scene word budgets (~10k target)
  write_story    per-scene authoring → assembled draft
  edit_story     quality / cut pass (+ continuity/fog verify loop)
  canon_delta    atomic facts → canon.md
  summarize      200-400 word recap → priors_file for next call
  compose_cover  SDXL prompt for installment cover
  generate_cover ComfyUI runs the SDXL workflow
  showrunner     paragraphs → narration script
  cast_voice     Kokoro per-segment TTS
  mix_episode    concat + loudnorm → episode.m4b (with cover + metadata)

--best-of N generates the prose N times and ships the lowest-badness
convergence; narration runs once, on the winner.

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
			if err := validateNarrator(storyNarrator); err != nil {
				return err
			}
			return runStory(cmd, slug)
		},
	}
	cmd.Flags().BoolVar(&regenForce, "force", false, "Allow regenerating an installment whose episode.m4b already exists (overwrites it and truncates its canon).")
	cmd.Flags().StringVar(&storySlug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().IntVar(&storyInstallment, "installment", 0, "Specific installment number to (re)generate. 0 = next pending.")
	cmd.Flags().StringVar(&storyNarrator, "narrator", "am_fenrir", "Kokoro voice id for the narrator.")
	cmd.Flags().StringVar(&storyPublishTo, "publish-to", "", "Directory to copy the finished episode.m4b into.")
	cmd.Flags().IntVar(&storyBestOf, "best-of", 1, "Generate the prose N times and ship the lowest-badness convergence (narration runs once, on the winner). 1 = single pass.")
	return cmd
}

func novelCommand() *cobra.Command {
	var (
		slug           string
		targetChapters int
		narrator       string
		publishTo      string
		bestOf         int
		force          bool
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
			if err := validateNarrator(narrator); err != nil {
				return err
			}
			// generateInstallment reads the package-level best-of
			// count; honour novel's own flag.
			storyBestOf = bestOf
			regenForce = force
			return runNovel(cmd, slug, targetChapters, narrator, publishTo)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().IntVar(&targetChapters, "target-chapters", 0, "Max chapters to generate this run (0 = all in arc.json).")
	cmd.Flags().StringVar(&narrator, "narrator", "am_fenrir", "Kokoro voice id for the narrator.")
	cmd.Flags().StringVar(&publishTo, "publish-to", "", "Directory to copy the finished book.m4b into.")
	cmd.Flags().IntVar(&bestOf, "best-of", 1, "Per-chapter prose attempts; ships the lowest-badness convergence (narration runs once, on the winner). 1 = single pass.")
	cmd.Flags().BoolVar(&force, "force", false, "Regenerate chapters whose episode.m4b already exists (overwrites them and truncates their canon).")
	return cmd
}

func runNovel(cmd *cobra.Command, slug string, targetChapters int, narrator, publishTo string) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	unlock, err := lockWorld(layout)
	if err != nil {
		return err
	}
	defer unlock()

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
	// A series-mode arc.json (from `series plan`) nests its chapters under "books"
	// and leaves the flat "chapters" empty. `novel` reads only the flat list, so it
	// would otherwise silently treat a fully-planned series as an empty novel. Detect
	// that shape and point the user at the right command instead of producing nothing.
	if len(arc.Chapters) == 0 && len(arc.Books) > 0 {
		return fmt.Errorf("arc.json at %s is a multi-book series (chapters are nested under \"books\") — use `worldsmith series write %s`, not `novel`", layout.ArcFile(), slug)
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
		// Resume: a finished chapter already has its episode.m4b. --force
		// regenerates it instead of skipping.
		if _, err := os.Stat(layout.InstallmentFile(i, "episode.m4b")); err == nil && !regenForce {
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
		// A --publish-to user still expects audio out. With no book.m4b to
		// copy, fall back to publishing the per-chapter episodes so the run
		// doesn't exit zero with nothing delivered.
		if publishTo != "" {
			published, perr := publishChapters(cmd, layout, count, publishTo)
			if perr != nil {
				return perr
			}
			if published == 0 {
				return fmt.Errorf("assemble book: %w", err)
			}
			return nil
		}
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
	tmp, err := os.MkdirTemp("", "bookasm")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	listPath := filepath.Join(tmp, "book_concat.txt")
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
	c := exec.CommandContext(cmd.Context(), ff, "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", bookPath)
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

func briefCommand() *cobra.Command {
	var (
		slug        string
		installment int
		steer       string
		targetWords int
		force       bool
	)
	cmd := &cobra.Command{
		Use:   "brief <slug>",
		Short: "Draft the next installment's brief from where the story stands.",
		Long: `brief reads the world bible, canon, and prior summaries and drafts
the next installment's briefs/NNN.md — the human direction document
the story pipeline consumes. It writes a DRAFT for you to edit; it
never runs the story off it.

--steer "<one line>" focuses the installment; omit it to let the model
free-run the arc. --installment N targets a specific number (default:
the next number without a brief). Refuses to overwrite an existing
brief unless --force.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required (positional arg or --slug)")
			}
			return runBrief(cmd, slug, installment, steer, targetWords, force)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().IntVar(&installment, "installment", 0, "Installment number to draft (0 = next without a brief).")
	cmd.Flags().StringVar(&steer, "steer", "", "Optional one-line direction for this installment.")
	cmd.Flags().IntVar(&targetWords, "target-words", 6500, "Target prose length the beats should sum to.")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing brief.")
	return cmd
}

func runBrief(cmd *cobra.Command, slug string, installment int, steer string, targetWords int, force bool) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	unlock, err := lockWorld(layout)
	if err != nil {
		return err
	}
	defer unlock()

	if _, err := os.Stat(layout.WorldFile()); err != nil {
		return fmt.Errorf("world.md not found at %s — run `worldsmith init %s` first", layout.WorldFile(), slug)
	}

	n := installment
	if n == 0 {
		n, err = world.NextBriefNumber(layout)
		if err != nil {
			return err
		}
	}
	briefPath := layout.BriefFile(n)
	if !force {
		if _, err := os.Stat(briefPath); err == nil {
			return fmt.Errorf("brief %03d already exists at %s — edit it, or pass --force to regenerate", n, briefPath)
		}
	}

	canonPath, err := world.EnsureCanonFile(layout)
	if err != nil {
		return fmt.Errorf("ensure canon: %w", err)
	}
	genDir := filepath.Join(layout.BriefsDir(), ".gen", fmt.Sprintf("%03d", n))
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		return err
	}
	priorsPath, err := world.EnsurePriorsFile(layout, genDir, n)
	if err != nil {
		return fmt.Errorf("ensure priors: %w", err)
	}
	timeline, err := world.LoadTimeline(layout)
	if err != nil {
		return fmt.Errorf("load timeline: %w", err)
	}
	histPath, err := world.WriteHistoricalContext(genDir, timeline.Events,
		world.FilterOpts{YearCutoff: timeline.Calendar.CurrentYear, HasCutoff: true})
	if err != nil {
		return fmt.Errorf("write historical context: %w", err)
	}
	exemplar := os.DevNull
	if last := world.LatestBriefNumber(layout); last > 0 && last != n {
		exemplar = layout.BriefFile(last)
	}
	notebookPath, err := world.WriteAssembledNotebook(layout, genDir)
	if err != nil {
		return fmt.Errorf("assemble notebook: %w", err)
	}

	cfg := pipeline.BriefConfig{
		InstallmentNumber:     n,
		TargetWords:           targetWords,
		Steer:                 steer,
		WorldFile:             layout.WorldFile(),
		CharactersFile:        layout.CharactersFile(),
		CanonFile:             canonPath,
		PriorsFile:            priorsPath,
		HistoricalContextFile: histPath,
		ExemplarBriefFile:     exemplar,
		NotebookFile:          notebookPath,
	}

	fmt.Fprintf(cmd.OutOrStdout(), "world: %s\n", slug)
	fmt.Fprintf(cmd.OutOrStdout(), "drafting brief %03d (target %d words)...\n\n", n, targetWords)

	root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
		return pipeline.BuildBrief(cfg)
	})
	if err != nil {
		return err
	}
	root.SetArgs([]string{"run", "--run-dir", genDir, "--no-cache"})
	if err := root.ExecuteContext(cmd.Context()); err != nil {
		return fmt.Errorf("brief %d: %w", n, err)
	}

	raw, err := os.ReadFile(filepath.Join(genDir, "brief.md"))
	if err != nil {
		return fmt.Errorf("read generated brief: %w", err)
	}
	if err := os.WriteFile(briefPath, raw, 0o644); err != nil {
		return fmt.Errorf("write brief: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ draft brief written: %s\n", briefPath)
	fmt.Fprintf(cmd.OutOrStdout(), "  review/edit it, then: worldsmith story %s\n", slug)
	return nil
}

func arcCommand() *cobra.Command {
	var (
		slug     string
		premise  string
		chapters int
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "arc <slug>",
		Short: "Draft a novel's chapter arc (arc.json) from a scope premise.",
		Long: `arc reads the world bible and an optional scope premise and drafts
arc.json — the ordered chapter beats that worldsmith novel runs, one
chapter at a time. It writes a DRAFT for you to edit; it never runs
the novel off it.

--premise "<scope>" sets the book's spine; omit it to let the model
propose the arc from the world alone. --chapters N sets the target
length. Refuses to overwrite an existing arc.json unless --force.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required (positional arg or --slug)")
			}
			return runArc(cmd, slug, premise, chapters, force)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().StringVar(&premise, "premise", "", "Optional scope/premise for the whole novel.")
	cmd.Flags().IntVar(&chapters, "chapters", 12, "Approximate chapter count.")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing arc.json.")
	return cmd
}

func runArc(cmd *cobra.Command, slug, premise string, chapters int, force bool) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	unlock, err := lockWorld(layout)
	if err != nil {
		return err
	}
	defer unlock()

	if _, err := os.Stat(layout.WorldFile()); err != nil {
		return fmt.Errorf("world.md not found at %s — run `worldsmith init %s` first", layout.WorldFile(), slug)
	}
	if !force {
		if _, err := os.Stat(layout.ArcFile()); err == nil {
			return fmt.Errorf("arc.json already exists at %s — edit it, or pass --force to regenerate", layout.ArcFile())
		}
	}
	canonPath, err := world.EnsureCanonFile(layout)
	if err != nil {
		return fmt.Errorf("ensure canon: %w", err)
	}
	genDir := filepath.Join(layout.Root, ".gen-arc")
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		return err
	}
	cfg := pipeline.ArcConfig{
		Premise:        premise,
		TargetChapters: chapters,
		WorldFile:      layout.WorldFile(),
		CharactersFile: layout.CharactersFile(),
		CanonFile:      canonPath,
	}
	fmt.Fprintf(cmd.OutOrStdout(), "world: %s\n", slug)
	fmt.Fprintf(cmd.OutOrStdout(), "drafting arc (~%d chapters)...\n\n", chapters)

	root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
		return pipeline.BuildArc(cfg)
	})
	if err != nil {
		return err
	}
	root.SetArgs([]string{"run", "--run-dir", genDir, "--no-cache"})
	if err := root.ExecuteContext(cmd.Context()); err != nil {
		return fmt.Errorf("arc: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(genDir, "arc.json"))
	if err != nil {
		return fmt.Errorf("read generated arc: %w", err)
	}
	if err := os.WriteFile(layout.ArcFile(), raw, 0o644); err != nil {
		return fmt.Errorf("write arc.json: %w", err)
	}
	if a, ok, perr := world.LoadArc(layout); perr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: generated arc.json didn't parse cleanly — edit it before running novel: %v\n", perr)
	} else if ok {
		fmt.Fprintf(cmd.OutOrStdout(), "  %d chapters: %s\n", len(a.Chapters), fallbackStr(a.Title, "(untitled)"))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ draft arc written: %s\n", layout.ArcFile())
	fmt.Fprintf(cmd.OutOrStdout(), "  review/edit it, then: worldsmith novel %s\n", slug)
	return nil
}

// seriesCommand groups the multi-book series flow: `plan` drafts the per-book
// chapter beats (arc.json) from series.json; `write` generates the chapters and
// assembles a chaptered .m4b per book.
func seriesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "series",
		Short: "Plan and generate a multi-book novel series.",
		Long: `series turns a hand-authored series.json (a narrative arc + an ordered
list of books, each with a premise, length, POV roster, and reveal-license)
into a trilogy of audiobooks.

  worldsmith series plan <slug>    Draft arc.json: per-book chapter beats.
  worldsmith series write <slug>   Generate chapters → one chaptered .m4b per book.

The per-chapter pipeline (per-scene authoring, verify-loop, fog/continuity,
canon/priors) is the same one 'story' and 'novel' use; series adds the
book-level planning, per-book reveal-pacing, and chaptered-m4b assembly.`,
	}
	cmd.AddCommand(seriesPlanCommand())
	cmd.AddCommand(seriesWriteCommand())
	return cmd
}

func seriesPlanCommand() *cobra.Command {
	var slug string
	var force bool
	cmd := &cobra.Command{
		Use:   "plan <slug>",
		Short: "Draft arc.json (per-book chapter beats) from series.json.",
		Long: `plan reads series.json + the world bible + canon + sealed notebook and
drafts each book's chapter beats into arc.json (a books[] of chapter beats,
each beat POV- and reveal-tagged). It writes a DRAFT for you to edit; it never
runs the story off it. If series.json is missing, plan writes a stub for you to
fill in.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required (positional arg or --slug)")
			}
			return runSeriesPlan(cmd, slug, force)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite an existing arc.json.")
	return cmd
}

func runSeriesPlan(cmd *cobra.Command, slug string, force bool) error {
	out := cmd.OutOrStdout()
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	unlock, err := lockWorld(layout)
	if err != nil {
		return err
	}
	defer unlock()

	if _, err := os.Stat(layout.WorldFile()); err != nil {
		return fmt.Errorf("world.md not found at %s — run `worldsmith init %s` first", layout.WorldFile(), slug)
	}
	series, ok, err := world.LoadSeries(layout)
	if err != nil {
		return err
	}
	if !ok {
		if err := world.ScaffoldSeries(layout); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote a series.json stub at %s\n  fill in the arc + books, then re-run: worldsmith series plan %s\n", layout.SeriesFile(), slug)
		return nil
	}
	if len(series.Books) == 0 {
		return fmt.Errorf("series.json at %s has no books", layout.SeriesFile())
	}
	if !force {
		if _, err := os.Stat(layout.ArcFile()); err == nil {
			return fmt.Errorf("arc.json already exists at %s — edit it, or pass --force to regenerate", layout.ArcFile())
		}
	}
	canonPath, err := world.EnsureCanonFile(layout)
	if err != nil {
		return fmt.Errorf("ensure canon: %w", err)
	}
	genDir := filepath.Join(layout.Root, ".gen-series")
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		return err
	}
	notebookPath, err := world.WriteAssembledNotebook(layout, genDir)
	if err != nil {
		return fmt.Errorf("assemble notebook: %w", err)
	}

	books := append([]world.SeriesBook(nil), series.Books...)
	sort.Slice(books, func(i, j int) bool { return books[i].N < books[j].N })

	seriesArc := renderSeriesArc(series.Arc)
	arc := world.Arc{Title: series.SeriesTitle}
	var prior strings.Builder

	fmt.Fprintf(out, "series: %s — planning %d books\n\n", series.SeriesTitle, len(books))
	for _, b := range books {
		fmt.Fprintf(out, "book %d (%s): outlining ~%d chapters...\n", b.N, fallbackStr(b.Title, "untitled"), b.TargetChapters)
		cfg := pipeline.BookOutlineConfig{
			SeriesTitle:       series.SeriesTitle,
			SeriesArc:         seriesArc,
			BookN:             b.N,
			BookTitle:         b.Title,
			Premise:           b.Premise,
			ArcSummary:        b.ArcSummary,
			TargetChapters:    b.TargetChapters,
			POVRoster:         b.POVRoster,
			Reveals:           b.Reveals,
			PriorBooksSummary: prior.String(),
			WorldFile:         layout.WorldFile(),
			CharactersFile:    layout.CharactersFile(),
			CanonFile:         canonPath,
			NotebookFile:      notebookPath,
		}
		if err := runPipeline(cmd, genDir, func() (*vamp.Pipeline, error) {
			return pipeline.BuildBookOutline(cfg)
		}); err != nil {
			return fmt.Errorf("book %d outline: %w", b.N, err)
		}
		raw, err := os.ReadFile(filepath.Join(genDir, fmt.Sprintf("book_%02d_outline.json", b.N)))
		if err != nil {
			return fmt.Errorf("read book %d outline: %w", b.N, err)
		}
		var parsed struct {
			Chapters []world.ArcBeat `json:"chapters"`
		}
		if err := json.Unmarshal(world.StripJSONFence(raw), &parsed); err != nil {
			return fmt.Errorf("parse book %d outline: %w", b.N, err)
		}
		if len(parsed.Chapters) == 0 {
			return fmt.Errorf("book %d outline produced no chapters", b.N)
		}
		arc.Books = append(arc.Books, world.ArcBook{
			N:           b.N,
			Title:       b.Title,
			Premise:     b.Premise,
			Reveals:     b.Reveals,
			TargetWords: b.TargetWordsPerChapter,
			Chapters:    parsed.Chapters,
		})
		fmt.Fprintf(out, "  → %d chapters\n", len(parsed.Chapters))
		fmt.Fprintf(&prior, "Book %d (%s): %s\n", b.N, fallbackStr(b.Title, ""), b.ArcSummary)
	}

	data, err := json.MarshalIndent(arc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(layout.ArcFile(), data, 0o644); err != nil {
		return fmt.Errorf("write arc.json: %w", err)
	}
	total := 0
	for _, bk := range arc.Books {
		total += len(bk.Chapters)
	}
	fmt.Fprintf(out, "\n✓ drafted arc.json: %d books, %d chapters total\n  %s\n", len(arc.Books), total, layout.ArcFile())
	fmt.Fprintf(out, "  review/edit it, then: worldsmith series write %s\n", slug)
	return nil
}

func seriesWriteCommand() *cobra.Command {
	var slug, narrator, publishTo string
	var book, targetChapters, chaptersDeprecated int
	var force bool
	cmd := &cobra.Command{
		Use:   "write <slug>",
		Short: "Generate the series' chapters and assemble a chaptered .m4b per book.",
		Long: `write generates the planned chapters of a series from arc.json, in order,
assembling a chaptered .m4b per book.

--target-chapters caps how many chapters to generate this run within the
selected book (0 = all) — useful for slice tests. (The old name --chapters
still works as a hidden alias.)

--best-of N generates each chapter's prose N times and ships the
lowest-badness convergence (the fog/continuity verify-loop scores each
attempt); narration runs once, on the winner. 1 = single pass.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required (positional arg or --slug)")
			}
			if err := validateNarrator(narrator); err != nil {
				return err
			}
			// Honour the deprecated --chapters when the user set it and didn't also
			// pass the canonical --target-chapters.
			if cmd.Flags().Changed("chapters") && !cmd.Flags().Changed("target-chapters") {
				targetChapters = chaptersDeprecated
			}
			regenForce = force
			return runSeriesWrite(cmd, slug, book, targetChapters, narrator, publishTo)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().IntVar(&book, "book", 0, "Only generate this book number (0 = all books).")
	cmd.Flags().IntVar(&targetChapters, "target-chapters", 0, "Cap chapters generated this run within the selected book (0 = all). For slice tests.")
	// Deprecated alias for --target-chapters; kept working but hidden so existing
	// scripts don't break while the name converges with `novel --target-chapters`.
	cmd.Flags().IntVar(&chaptersDeprecated, "chapters", 0, "Deprecated alias for --target-chapters.")
	_ = cmd.Flags().MarkHidden("chapters")
	cmd.Flags().StringVar(&narrator, "narrator", "am_fenrir", "Kokoro narrator voice id.")
	cmd.Flags().StringVar(&publishTo, "publish-to", "", "Directory to copy finished book .m4b files into.")
	cmd.Flags().IntVar(&storyBestOf, "best-of", 1, "Generate each chapter's prose N times and ship the lowest-badness convergence (narration runs once, on the winner). 1 = single pass.")
	cmd.Flags().BoolVar(&force, "force", false, "Regenerate chapters whose episode.m4b already exists (overwrites them and truncates their canon).")
	return cmd
}

// renderSeriesArc renders the whole-series arc (key events + final state) for a
// planning prompt.
func renderSeriesArc(a world.SeriesArc) string {
	var b strings.Builder
	if len(a.KeyEvents) > 0 {
		b.WriteString("Key events the series must hit:\n")
		for _, e := range a.KeyEvents {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(e))
		}
	}
	if strings.TrimSpace(a.FinalState) != "" {
		fmt.Fprintf(&b, "\nFinal state: %s\n", strings.TrimSpace(a.FinalState))
	}
	if b.Len() == 0 {
		return "(no series arc specified)"
	}
	return b.String()
}

func runSeriesWrite(cmd *cobra.Command, slug string, bookFilter, chapterLimit int, narrator, publishTo string) error {
	out := cmd.OutOrStdout()
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	unlock, err := lockWorld(layout)
	if err != nil {
		return err
	}
	defer unlock()

	if _, err := os.Stat(layout.WorldFile()); err != nil {
		return fmt.Errorf("world.md not found — run `worldsmith init %s` first", slug)
	}
	arc, ok, err := world.LoadArc(layout)
	if err != nil {
		return err
	}
	if !ok || len(arc.Books) == 0 {
		return fmt.Errorf("no arc.json with books at %s — run `worldsmith series plan %s` first", layout.ArcFile(), slug)
	}
	series, _, serr := world.LoadSeries(layout) // optional, for book titles
	if serr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "note: series.json did not parse (%v) — falling back to arc titles\n", serr)
	}
	flat := arc.FlatChapters()

	// Compute each book's global chapter range [start..end] (1-based) by walking
	// the books in order.
	type bookRange struct {
		book       world.ArcBook
		start, end int
	}
	var ranges []bookRange
	idx := 0
	for _, b := range arc.Books {
		start := idx + 1
		end := idx + len(b.Chapters)
		idx = end
		ranges = append(ranges, bookRange{book: b, start: start, end: end})
	}

	for _, r := range ranges {
		if bookFilter > 0 && r.book.N != bookFilter {
			continue
		}
		// chapterLimit caps how many of this book's chapters we generate this run
		// (for slice tests). 0 = the whole book.
		genEnd := r.end
		if chapterLimit > 0 && r.start+chapterLimit-1 < genEnd {
			genEnd = r.start + chapterLimit - 1
		}
		fmt.Fprintf(out, "\n=== Book %d: %s (chapters %d–%d, generating through %d) ===\n",
			r.book.N, fallbackStr(bookTitleOf(series, r.book), "untitled"), r.start, r.end, genEnd)

		for n := r.start; n <= genEnd; n++ {
			beat := flat[n-1]
			// Materialize the chapter brief from the beat + the book's reveal-license
			// + target length — but only if absent, so a hand-edited brief and resume
			// both survive.
			briefPath := layout.BriefFile(n)
			if _, err := os.Stat(briefPath); os.IsNotExist(err) {
				// Per-chapter reveal pacing: a chapter that names its own reveals
				// (even an empty list) overrides the book-wide license; only an unset
				// (nil) chapter inherits the whole book's reveals. This stops a
				// high-reveal chapter from being handed the entire book's sealed
				// material and leaking scale it shouldn't show yet.
				rev := r.book.Reveals
				if beat.Reveals != nil {
					rev = *beat.Reveals
				}
				content := world.RenderSeriesChapterBrief(n, beat, rev, r.book.TargetWords)
				if err := os.WriteFile(briefPath, []byte(content), 0o644); err != nil {
					return fmt.Errorf("write chapter %d brief: %w", n, err)
				}
			}
			// Resume: a finished chapter has its episode.m4b. --force
			// regenerates it instead of skipping.
			if _, err := os.Stat(layout.InstallmentFile(n, "episode.m4b")); err == nil && !regenForce {
				fmt.Fprintf(out, "chapter %d (book %d): already done — skipping\n", n, r.book.N)
				continue
			}
			fmt.Fprintf(out, "\n--- chapter %d (book %d, %d/%d) ---\n", n, r.book.N, n-r.start+1, len(r.book.Chapters))
			if err := generateInstallment(cmd, layout, n, narrator); err != nil {
				return fmt.Errorf("chapter %d: %w", n, err)
			}
		}

		// Assemble the book's chaptered .m4b once its full range is complete.
		if genEnd != r.end {
			fmt.Fprintf(out, "book %d partially generated (slice); skipping assembly until complete\n", r.book.N)
			continue
		}
		complete := true
		for n := r.start; n <= r.end; n++ {
			if _, err := os.Stat(layout.InstallmentFile(n, "episode.m4b")); err != nil {
				complete = false
				break
			}
		}
		if !complete {
			continue
		}
		var nums []int
		var titles []string
		for n := r.start; n <= r.end; n++ {
			nums = append(nums, n)
			titles = append(titles, fallbackStr(flat[n-1].Title, fmt.Sprintf("Chapter %d", n-r.start+1)))
		}
		bookTitle := fmt.Sprintf("Book %d", r.book.N)
		if t := bookTitleOf(series, r.book); t != "" {
			bookTitle = fmt.Sprintf("Book %d — %s", r.book.N, t)
		}
		outPath := filepath.Join(layout.Root, fmt.Sprintf("book_%02d.m4b", r.book.N))
		if err := assembleBookChaptered(cmd, layout, nums, titles, bookTitle, outPath); err != nil {
			return fmt.Errorf("assemble book %d: %w", r.book.N, err)
		}
		fmt.Fprintf(out, "✓ assembled %s\n", outPath)
		if publishTo != "" {
			dst := filepath.Join(publishTo, sanitizeFilenameFragment(bookTitle)+".m4b")
			if err := copyFile(outPath, dst); err != nil {
				return fmt.Errorf("publish book %d: %w", r.book.N, err)
			}
			fmt.Fprintf(out, "  published: %s\n", dst)
		}
	}
	return nil
}

// bookTitleOf prefers the series.json book title, then the arc book title.
func bookTitleOf(s world.Series, b world.ArcBook) string {
	if sb, ok := s.Book(b.N); ok && strings.TrimSpace(sb.Title) != "" {
		return strings.TrimSpace(sb.Title)
	}
	return strings.TrimSpace(b.Title)
}

// assembleBookChaptered concatenates a book's chapter m4bs into one .m4b with
// real chapter markers (so Audiobookshelf shows the chapter list and resumes
// mid-chapter). It ffprobes each chapter's duration to compute cumulative
// chapter boundaries, writes an FFMETADATA file with [CHAPTER] blocks, and runs
// a single stream-copy ffmpeg pass that maps the chapters + a book title.
func assembleBookChaptered(cmd *cobra.Command, layout world.Layout, chapterNums []int, titles []string, bookTitle, outPath string) error {
	if len(chapterNums) == 0 {
		return fmt.Errorf("no chapters to assemble")
	}
	tmp, err := os.MkdirTemp("", "bookasm")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	// concat list + cumulative chapter times (ms).
	var concat strings.Builder
	var meta strings.Builder
	meta.WriteString(";FFMETADATA1\n")
	fmt.Fprintf(&meta, "title=%s\n", ffmetaEscape(bookTitle))
	meta.WriteString("genre=Audiobook\n")
	cursorMS := int64(0)
	for i, n := range chapterNums {
		ep := layout.InstallmentFile(n, "episode.m4b")
		if _, err := os.Stat(ep); err != nil {
			return fmt.Errorf("chapter %d m4b missing: %w", n, err)
		}
		fmt.Fprintf(&concat, "file '%s'\n", strings.ReplaceAll(ep, "'", `'\''`))
		durSec, err := probeDurationSec(cmd.Context(), ep)
		if err != nil {
			return fmt.Errorf("probe chapter %d: %w", n, err)
		}
		startMS := cursorMS
		endMS := cursorMS + int64(durSec*1000)
		cursorMS = endMS
		title := fmt.Sprintf("Chapter %d", i+1)
		if i < len(titles) && strings.TrimSpace(titles[i]) != "" {
			title = fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(titles[i]))
		}
		fmt.Fprintf(&meta, "[CHAPTER]\nTIMEBASE=1/1000\nSTART=%d\nEND=%d\ntitle=%s\n", startMS, endMS, ffmetaEscape(title))
	}
	concatPath := filepath.Join(tmp, "concat.txt")
	metaPath := filepath.Join(tmp, "ffmeta.txt")
	if err := os.WriteFile(concatPath, []byte(concat.String()), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(metaPath, []byte(meta.String()), 0o644); err != nil {
		return err
	}

	// One pass: concat-demux the chapter audio (stream copy) + take chapters and
	// global metadata from the FFMETADATA input.
	args := []string{
		"-y",
		"-f", "concat", "-safe", "0", "-i", concatPath,
		"-i", metaPath,
		// Audio only: the per-chapter m4bs carry an mjpeg cover stream the book
		// container rejects on copy; map just the audio. (A book-level cover can
		// be added separately later.)
		"-map", "0:a", "-map_metadata", "1", "-map_chapters", "1",
		"-c", "copy",
		outPath,
	}
	c := exec.CommandContext(cmd.Context(), "ffmpeg", args...)
	if outb, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg assemble: %w\n%s", err, lastLines(string(outb), 12))
	}
	return nil
}

// probeDurationSec returns a media file's duration in seconds via ffprobe.
func probeDurationSec(ctx context.Context, path string) (float64, error) {
	c := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path)
	outb, err := c.Output()
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(string(outb)), 64)
}

// ffmetaEscape escapes the FFMETADATA special characters (= ; # \ and newline).
func ffmetaEscape(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "=", "\\=", ";", "\\;", "#", "\\#", "\n", "\\\n")
	return r.Replace(s)
}

// lastLines returns the last n lines of s (for trimming ffmpeg output in errors).
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func runStory(cmd *cobra.Command, slug string) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}

	unlock, err := lockWorld(layout)
	if err != nil {
		return err
	}
	defer unlock()

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
// convergeProse runs ONE prose attempt for installment n: per-scene authoring → the
// edit-skipping Phase 1 → the verify-loop. It returns the path to the best (lowest-badness)
// converged prose IN the installment dir, its badness (fog leaks + breaking continuity), and
// the outline JSON. Narration is NOT run here — best-of-N picks a winner across attempts and
// narrates once. cfg is taken by value so an attempt's mutations don't leak back to the caller.
// NOTE: the returned path is a shared in-dir file (candidate_0.md / polished_N.md); the caller
// must copy it aside before the next attempt overwrites it.
// styleBadness is a deterministic scalar of the prose tells the scorecard
// penalises — slop hits, the not-X-but-Y reflex, and anaphora (summed over-used
// opener counts). Lower is better; used to keep a style-polish pass ONLY when it
// measurably improves the prose.
func styleBadness(text string) int {
	m := world.AnalyzeProse(text)
	b := m.SlopTotal + m.NotXButY
	for _, o := range m.RepeatedOpeners {
		b += o.Count
	}
	return b
}

// spliceReplacements applies a {replacements:[{span,replacement}]} document to the
// prose at prosePath by exact first-occurrence substring replacement (the same
// length-safe mechanism applyFixes uses for continuity), writing the result to
// outPath. Returns how many replacements landed. A span that no longer matches
// verbatim, is empty, or is unchanged is skipped.
func spliceReplacements(prosePath, replacementsPath, outPath string) (int, error) {
	prose, err := os.ReadFile(prosePath)
	if err != nil {
		return 0, err
	}
	raw, err := os.ReadFile(replacementsPath)
	if err != nil {
		return 0, err
	}
	var doc struct {
		Replacements []struct {
			Span        string `json:"span"`
			Replacement string `json:"replacement"`
		} `json:"replacements"`
	}
	if err := json.Unmarshal(world.StripJSONFence(raw), &doc); err != nil {
		return 0, err
	}
	s := string(prose)
	applied := 0
	for _, r := range doc.Replacements {
		span := strings.TrimSpace(r.Span)
		rep := strings.TrimSpace(r.Replacement)
		if span == "" || rep == "" || span == rep || !strings.Contains(s, span) {
			continue
		}
		s = strings.Replace(s, span, rep, 1)
		applied++
	}
	if err := os.WriteFile(outPath, []byte(s), 0o644); err != nil {
		return 0, err
	}
	return applied, nil
}

// stylePolish runs the metrics-driven prose-style remediation the per-scene flow
// otherwise lacks: extract the exact offending sentences (over-used openers, slop,
// not-X-but-Y), have the LLM recast ONLY those, splice the recasts back in, and
// keep the result only when measured style-badness drops. Bounded passes; returns
// the path to the best prose (the input unchanged when nothing improved). Best-effort
// — any failure ships the unpolished prose rather than breaking the chapter.
func stylePolish(cmd *cobra.Command, layout world.Layout, n int, installmentDir, prosePath, notebookFile, licensedRevealsFile string) string {
	const maxPasses = 2
	cur := prosePath
	for pass := 1; pass <= maxPasses; pass++ {
		raw, err := os.ReadFile(cur)
		if err != nil {
			return cur
		}
		text := string(raw)
		spans := world.OffendingSentences(text, 40)
		if len(spans) < 3 {
			break // already clean enough to leave alone
		}
		spansPath := layout.InstallmentFile(n, "style_spans.json")
		payload, err := json.Marshal(map[string]any{"sentences": spans})
		if err != nil {
			return cur
		}
		if err := os.WriteFile(spansPath, payload, 0o644); err != nil {
			return cur
		}
		if err := runPipeline(cmd, installmentDir, func() (*vamp.Pipeline, error) {
			return pipeline.BuildProsePolish(pipeline.ProsePolishConfig{
				SpansFile:           spansPath,
				NotebookFile:        notebookFile,
				LicensedRevealsFile: licensedRevealsFile,
			})
		}); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "style polish pass %d: %v (shipping unpolished)\n", pass, err)
			return cur
		}
		outPath := layout.InstallmentFile(n, fmt.Sprintf("style_polished_%d.md", pass))
		applied, err := spliceReplacements(cur, layout.InstallmentFile(n, "prose_polish.json"), outPath)
		if err != nil || applied == 0 {
			break
		}
		polished, err := os.ReadFile(outPath)
		if err != nil {
			break
		}
		before, after := styleBadness(text), styleBadness(string(polished))
		fmt.Fprintf(cmd.OutOrStdout(),
			"style polish pass %d: recast %d sentence(s), style-badness %d -> %d\n", pass, applied, before, after)
		if after >= before {
			break // no improvement — keep the prior best
		}
		cur = outPath
	}
	return cur
}

func convergeProse(cmd *cobra.Command, layout world.Layout, n int, cfg pipeline.StoryConfig, installmentDir string) (bestPath string, bestBad int, outlineJSON string, err error) {
	// Per-scene authoring: outline → write each scene as its own LLM pass (sequential,
	// each seeing the prior scenes) → stitch. This is how installments hit length —
	// per-scene word budgets the model honours, instead of one uncontrollable-length
	// pass — and each scene gets full attention.
	var draftFile string
	draftFile, outlineJSON, err = generatePerSceneDraft(cmd, layout, n, cfg, installmentDir)
	if err != nil {
		return "", 0, "", err
	}
	// Style-polish the stitched draft BEFORE the terminal checks + verify-loop. The
	// per-scene writer ships slop/anaphora that nothing downstream fixes (edit_story is
	// a pass-through in PreEdited mode), so recast the flagged sentences here — but do it
	// FIRST, so the fog/continuity verify-loop below then runs on the polished prose and
	// cuts any sealed-material leak a recast might introduce (recasting a sentence in
	// isolation can turn an oblique line into an explicit reveal). Length-safe + kept
	// only when measured style-badness drops.
	draftFile = stylePolish(cmd, layout, n, installmentDir, draftFile, cfg.NotebookFile, cfg.LicensedRevealsFile)
	cfg.DraftFile = draftFile
	cfg.OutlineJSON = outlineJSON

	// Phase 1: run the terminal checks on the stitched per-scene draft, but STOP before
	// narration — don't pay for TTS until the prose has converged.
	//
	// PreEdited=true makes edit_story a PASS-THROUGH here, deliberately. The per-scene
	// writer already produced an on-budget, scene-controlled draft (e.g. 6.1k words to a
	// 5.5k target); the whole-doc edit_story rewrite, asked to "preserve length," ignores
	// that instruction and silently compresses — observed crushing a 6.1k draft to 4.8k
	// (-22%). A whole-document rewrite treats the length rule as advisory and shrinks no
	// matter how forcefully the prompt forbids it. So we skip the rewrite and let the
	// verify-loop below do all fixing SURGICALLY (span-splice + deterministic fog-cut),
	// which preserves the draft's length by construction. Style/continuity/fog are handled
	// by the loop's checks; the per-scene write_scene pass carries the line-level polish.
	cfg.PreEdited = true
	cfg.SkipNarration = true
	if err := runPipeline(cmd, installmentDir, func() (*vamp.Pipeline, error) {
		return pipeline.BuildStory(cfg)
	}); err != nil {
		return "", 0, "", fmt.Errorf("installment %d (prose): %w", n, err)
	}

	// Phase 2: the verify-loop. While the terminal checks still flag a fog LEAK or a
	// BREAKING continuity error, run a targeted span-fix on exactly those spans and
	// re-check. Bounded so a stubborn finding can't loop forever. This makes the shipped
	// prose provably pass the same checks that score it, instead of trusting one edit.
	prosePath := layout.InstallmentFile(n, "story.md")
	fogRpt := layout.InstallmentFile(n, "fog_report.md")
	contRpt := layout.InstallmentFile(n, "continuity_report.md")
	// Monotonic guard: the checkers are noisy and a fix pass can make things WORSE
	// (deletions leave seams, rewrites add claims — observed continuity 2→11). So keep the
	// BEST version seen (fewest leaks + breaking) and ship THAT, never merely the last.
	// Phase 3 re-checks the chosen prose, so the final reports + scorecard match what ships.
	badness := func() int { return verdictCount(fogRpt, "LEAK") + verdictCount(contRpt, "BREAKING") }
	// Audit both reports (drop phantom not-in-prose findings + adversarially verify
	// continuity) before scoring — so badness() (and thus best-of-N + the loop) tracks
	// true, in-prose, canon-backed breaks, not the 27B checkers' over-flags/hallucinations.
	auditFindings(cmd, layout, n, installmentDir, prosePath, fogRpt, contRpt, cfg.WorldFile, cfg.CanonFile)
	bestPath = layout.InstallmentFile(n, "candidate_0.md")
	if err := copyFile(prosePath, bestPath); err != nil {
		return "", 0, "", fmt.Errorf("installment %d stage candidate: %w", n, err)
	}
	bestBad = badness()
	const maxFixPasses = 3
	for iter := 1; iter <= maxFixPasses && bestBad > 0; iter++ {
		fmt.Fprintf(cmd.OutOrStdout(),
			"verify-loop pass %d: %d fog leak(s), %d breaking continuity (best so far: %d) — applying span fixes\n",
			iter, verdictCount(fogRpt, "LEAK"), verdictCount(contRpt, "BREAKING"), bestBad)
		// Span-level fix: ask the model ONLY for replacement text for each flagged span
		// (it never sees the full prose, so it can't lapse into copy-the-document), then
		// splice the replacements in deterministically here in Go.
		scfg := pipeline.SpanFixConfig{
			FogReportFile:        fogRpt,
			ContinuityReportFile: contRpt,
			WorldFile:            cfg.WorldFile,
			NotebookFile:         cfg.NotebookFile,
			LicensedRevealsFile:  cfg.LicensedRevealsFile,
			BriefFile:            cfg.BriefFile,
			OutputName:           "spanfix.json",
		}
		if err := runPipeline(cmd, installmentDir, func() (*vamp.Pipeline, error) {
			return pipeline.BuildSpanFix(scfg)
		}); err != nil {
			return "", 0, "", fmt.Errorf("installment %d span-fix pass %d: %w", n, iter, err)
		}
		polishedName := fmt.Sprintf("polished_%d.md", iter)
		applied, err := applyFixes(
			prosePath,
			layout.InstallmentFile(n, "spanfix.json"),
			fogRpt,
			layout.InstallmentFile(n, polishedName),
		)
		if err != nil {
			return "", 0, "", fmt.Errorf("installment %d apply-fixes pass %d: %w", n, iter, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  applied %d fix(es) (fog cuts + continuity rewrites)\n", applied)
		// Re-verify: pass the spliced prose straight through (PreEdited) and re-run the
		// terminal checks on it, refreshing the reports for the next loop test.
		vcfg := cfg
		vcfg.DraftFile = layout.InstallmentFile(n, polishedName)
		vcfg.PreEdited = true
		vcfg.SkipNarration = true
		if err := runPipeline(cmd, installmentDir, func() (*vamp.Pipeline, error) {
			return pipeline.BuildStory(vcfg)
		}); err != nil {
			return "", 0, "", fmt.Errorf("installment %d re-verify pass %d: %w", n, iter, err)
		}
		prosePath = layout.InstallmentFile(n, "story.md") // re-verify wrote the polished prose here
		// Re-audit the refreshed findings before re-scoring this pass.
		auditFindings(cmd, layout, n, installmentDir, prosePath, fogRpt, contRpt, cfg.WorldFile, cfg.CanonFile)
		if bad := badness(); bad < bestBad {
			bestBad = bad
			bestPath = layout.InstallmentFile(n, polishedName)
			fmt.Fprintf(cmd.OutOrStdout(), "  new best: %d unresolved finding(s)\n", bad)
		}
	}
	if bestBad > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"verify-loop: best achievable was %d unresolved finding(s) (fog leak + breaking continuity) — shipping the best version; see reports\n",
			bestBad)
	}
	return bestPath, bestBad, outlineJSON, nil
}

func generateInstallment(cmd *cobra.Command, layout world.Layout, n int, narrator string) error {
	// Regenerating over a finished installment destroys its episode.m4b and
	// truncates its canon section — both irreversible. Refuse unless the
	// caller passed --force. The check precedes TruncateCanonFrom so a
	// declined regen leaves the prior run fully intact.
	if !regenForce {
		if _, err := os.Stat(layout.InstallmentFile(n, "episode.m4b")); err == nil {
			return fmt.Errorf("installment %d already finished at %s — pass --force to regenerate (overwrites the episode and truncates its canon)",
				n, layout.InstallmentFile(n, "episode.m4b"))
		}
	}
	canonPath, err := world.EnsureCanonFile(layout)
	if err != nil {
		return fmt.Errorf("ensure canon: %w", err)
	}
	// (Re)generating installment n: drop ONLY n's own prior canon section so the writer
	// reads a clean ledger, never a stale self-extraction from a scrapped run. Every
	// other installment's canon (earlier AND later) is kept — regenerating a single
	// installment must not destroy canon the rest of the work depends on. The writer
	// view is additionally scoped to "as of n" by WriteRelevantCanon; the post-run
	// AppendCanonDelta re-inserts n's fresh section in order.
	if err := world.TruncateCanonFrom(layout, n); err != nil {
		return fmt.Errorf("truncate canon: %w", err)
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
		installmentDir, canonPath, briefBody, brief.OnStageActors, world.DefaultCanonBudget, n)
	if err != nil {
		return fmt.Errorf("filter canon: %w", err)
	}

	// Assemble the author's private notebook (accepted dossiers) into the run
	// dir. The outline + writer read it for depth/foreshadowing under fog-of-war;
	// empty file when the world has no notebook yet.
	notebookPath, err := world.WriteAssembledNotebook(layout, installmentDir)
	if err != nil {
		return fmt.Errorf("assemble notebook: %w", err)
	}

	// Render this installment's licensed-reveal allow-list from the brief's
	// `reveals:` frontmatter — the sealed notebook material permitted onto the page
	// this installment. The writer + fog-check read it; "none" when nothing licensed.
	revealsPath, err := world.WriteLicensedReveals(installmentDir, brief.Reveals)
	if err != nil {
		return fmt.Errorf("write licensed reveals: %w", err)
	}

	// Grounding pass: extract the exact canon + mechanics this chapter's events touch
	// into chapter_facts.md, pinned into the writer so it doesn't improvise dense canon
	// (the continuity failure mode on high-canon-density chapters like the contact event).
	if err := runPipeline(cmd, installmentDir, func() (*vamp.Pipeline, error) {
		return pipeline.BuildChapterFacts(pipeline.ChapterFactsConfig{
			BriefFile:             layout.BriefFile(n),
			WorldFile:             layout.WorldFile(),
			CanonFile:             canonPath,
			NotebookFile:          notebookPath,
			PriorsFile:            priorsPath,
			HistoricalContextFile: historyPath,
		})
	}); err != nil {
		return fmt.Errorf("chapter facts: %w", err)
	}
	chapterFactsPath := layout.InstallmentFile(n, "chapter_facts.md")

	cfg := pipeline.StoryConfig{
		InstallmentNumber:     n,
		WorldFile:             layout.WorldFile(),
		CharactersFile:        layout.CharactersFile(),
		CanonFile:             canonPath,
		CanonRelevantFile:     canonRelevantPath,
		PriorsFile:            priorsPath,
		BriefFile:             layout.BriefFile(n),
		HistoricalContextFile: historyPath,
		NotebookFile:          notebookPath,
		LicensedRevealsFile:   revealsPath,
		ChapterFactsFile:      chapterFactsPath,
		NarratorVoice:         narrator,
		TargetWords:           brief.TargetWords,
		// Phase the cover out: the prose runs with the LLM resident, then the CLI
		// frees the LLM and renders the cover + mix separately (see below). A 28GB
		// EXL3 leaves a 32GB card no room for the cover model in a single pass.
		SkipFinalize:    true,
		CoverPromptFile: layout.InstallmentFile(n, "cover_prompt.txt"),
	}

	fmt.Fprintf(cmd.OutOrStdout(), "installment: %d\n", n)
	fmt.Fprintf(cmd.OutOrStdout(), "brief: %s\n", layout.BriefFile(n))
	fmt.Fprintln(cmd.OutOrStdout(), "")
	logVRAM(cmd, fmt.Sprintf("installment %d start", n))

	// Best-of-N: prose generation is stochastic — the same engine yields a few different
	// subtle findings each run (observed 3 one run, 7 the next, same chapter). So generate
	// the prose up to storyBestOf times and keep the lowest-badness convergence; the
	// expensive narration (TTS + mix) runs ONCE, on the winner. N=1 is the old single pass.
	attempts := storyBestOf
	if attempts < 1 {
		attempts = 1
	}
	bestConverged, bestOutline := "", ""
	bestBad := -1
	for i := 1; i <= attempts; i++ {
		attemptCfg := cfg
		if attempts > 1 {
			fmt.Fprintf(cmd.OutOrStdout(), "\nbest-of-%d: prose attempt %d/%d\n", attempts, i, attempts)
			// Distinct seed per attempt → fresh sampling (temp 0.8) + distinct cache
			// keys, so the attempts actually differ instead of replaying the cache.
			attemptCfg.Seed = i
		}
		winPath, bad, outlineJSON, err := convergeProse(cmd, layout, n, attemptCfg, installmentDir)
		if err != nil {
			return err
		}
		if attempts == 1 {
			bestConverged, bestOutline, bestBad = winPath, outlineJSON, bad
			break
		}
		// Preserve this attempt's winning prose before the next attempt overwrites the
		// shared in-dir files (story.md, candidate_0.md, polished_*.md).
		attemptPath := layout.InstallmentFile(n, fmt.Sprintf("attempt_%d.md", i))
		if err := copyFile(winPath, attemptPath); err != nil {
			return fmt.Errorf("installment %d preserve attempt %d: %w", n, i, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "best-of-%d: attempt %d scored %d unresolved finding(s)\n", attempts, i, bad)
		if bestBad < 0 || bad < bestBad {
			bestBad, bestConverged, bestOutline = bad, attemptPath, outlineJSON
		}
		if bestBad == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "best-of-%d: attempt %d is clean (0 findings) — stopping early\n", attempts, i)
			break
		}
	}
	if attempts > 1 {
		fmt.Fprintf(cmd.OutOrStdout(), "best-of-%d: shipping the cleanest attempt (%d unresolved finding(s))\n", attempts, bestBad)
	}

	// Phase 3: narrate + finalize on the BEST prose. Copy it to a stable file so the
	// pipeline reads it as a DraftFile (PreEdited = no second rewrite). Phase 3 re-runs
	// the checks, so the shipped reports + scorecard describe this exact prose.
	convergedPath := layout.InstallmentFile(n, "converged.md")
	if err := copyFile(bestConverged, convergedPath); err != nil {
		return fmt.Errorf("installment %d stage converged prose: %w", n, err)
	}
	cfg.DraftFile = convergedPath
	cfg.OutlineJSON = bestOutline
	cfg.PreEdited = true
	cfg.SkipNarration = false
	// LLM is resident here; the narration pipeline frees it (FreeProfileAfter on
	// showrunner) BEFORE cast_voice/TTS runs — so a healthy run shows VRAM fall
	// mid-pipeline rather than holding LLM+TTS together (the old freeze).
	logVRAM(cmd, fmt.Sprintf("ch%d before narration (LLM resident)", n))
	if err := runPipeline(cmd, installmentDir, func() (*vamp.Pipeline, error) {
		return pipeline.BuildStory(cfg)
	}); err != nil {
		return fmt.Errorf("installment %d (narration): %w", n, err)
	}
	logVRAM(cmd, fmt.Sprintf("ch%d after narration (TTS done)", n))
	// Phase 3 re-ran the raw checks; audit them once more so the SHIPPED reports +
	// scorecard describe the real (in-prose, canon-backed) findings, not the checkers'
	// raw over-flags / notebook hallucinations.
	auditFindings(cmd, layout, n, installmentDir,
		layout.InstallmentFile(n, "story.md"),
		layout.InstallmentFile(n, "fog_report.md"),
		layout.InstallmentFile(n, "continuity_report.md"),
		cfg.WorldFile, cfg.CanonFile)

	// Narration + cover prompt ran with the LLM resident. Free it so the cover phase has
	// VRAM for ComfyUI, then render the cover + mix the m4b.
	freeActiveLLM(cmd, "the cover phase")
	root2, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
		return pipeline.BuildEpisodeFinalize(cfg)
	})
	if err != nil {
		return err
	}
	root2.SetArgs([]string{"run", "--run-dir", installmentDir})
	if err := root2.ExecuteContext(cmd.Context()); err != nil {
		return fmt.Errorf("installment %d finalize (cover + mix): %w", n, err)
	}

	// Post-run: fold this installment's canon_delta into the
	// running canon.md so the next call reads it.
	if err := world.AppendCanonDelta(layout, n); err != nil {
		return fmt.Errorf("append canon: %w", err)
	}

	// Deterministic prose-health read: slop density, the not-X-but-Y
	// reflex, anaphora, repeated trigrams. Written to metrics.json
	// beside the m4b. Non-fatal — a clean run shouldn't break because
	// the audit couldn't run. The analysis is reused for the scorecard
	// below so story.md is read + analysed once, not twice.
	var prose *world.ProseMetrics
	if m, err := world.WriteProseMetrics(
		layout.InstallmentFile(n, "story.md"),
		layout.InstallmentFile(n, "metrics.json"),
	); err == nil {
		prose = &m
		fmt.Fprintf(cmd.OutOrStdout(),
			"prose: %d words, slop %.1f/1k (%d hits), not-x-but-y %d\n",
			m.Words, m.SlopPer1000, m.SlopTotal, m.NotXButY)
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: prose metrics: %v\n", err)
	}

	// Quality scorecard: prose + continuity rolled into one tracked number
	// (scorecard.json) so quality is diffable across installments, not a vibe.
	if card, err := world.WriteScorecard(layout, n, prose); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "scorecard: overall %d/100\n", world.Overall(card))
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: scorecard: %v\n", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ installment %d done: %s\n", n, layout.InstallmentFile(n, "episode.m4b"))
	return nil
}

// logVRAM prints current GPU VRAM usage with a label. It exists to make the
// narration-phase VRAM pressure that once hard-locked the box observable in the
// run log: a healthy chapter shows VRAM fall from ~LLM-resident down to TTS-only
// across the narration phase (the FreeProfileAfter unload) and again after
// freeActiveLLM, never climbing toward the card's ceiling. Best-effort — silent
// when nvidia-smi isn't on PATH (e.g. CI), so it never affects a run's outcome.
func logVRAM(cmd *cobra.Command, label string) {
	used, err := gpuUsedMiB(cmd.Context())
	if err != nil {
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  [vram] %-40s %5d MiB used\n", label, used)
}

// freeActiveLLM unloads the active vibe LLM profile (best-effort) so the next phase has
// VRAM for ComfyUI — a large resident LLM (e.g. the 28GB EXL3) otherwise leaves a 32GB
// card no room for even the small SDXL cover model or the scene image/video models.
// Services (comfyui, kokoro) stay up; the next pipeline that needs the LLM re-activates
// it via its capability. purpose labels the run log. Non-fatal: a failure just risks an
// OOM downstream if VRAM is tight.
func freeActiveLLM(cmd *cobra.Command, purpose string) {
	fmt.Fprintf(cmd.OutOrStdout(), "freeing the LLM profile for %s (vibe stop)...\n", purpose)
	// Detach from cmd.Context(): freeing VRAM is the cleanup we want to run
	// *because* of a Ctrl-C, so it must not inherit the cancelled context.
	// A bounded timeout still prevents a hung `vibe stop` from blocking exit.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "vibe", "stop")
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: could not free the LLM (vibe stop: %v) — %s may OOM if VRAM is tight\n", err, purpose)
	}
	logVRAM(cmd, "after freeActiveLLM (LLM unloaded)")
}

// runPipeline builds and executes a vamp pipeline in runDir. Shared by the per-scene
// authoring steps (the outline + each scene), which each run in their own run-dir so
// the vamp cache doesn't collide on shared stage names across scenes.
func runPipeline(cmd *cobra.Command, runDir string, build func() (*vamp.Pipeline, error)) error {
	root, err := vamp.BuildRoot(build)
	if err != nil {
		return err
	}
	root.SetArgs([]string{"run", "--run-dir", runDir})
	// ExecuteContext so a Ctrl-C / SIGTERM during a sub-pipeline run cancels
	// the vamp executor (and the model/ffmpeg work it drives) instead of
	// orphaning it.
	return root.ExecuteContext(cmd.Context())
}

// leakRe / breakingRe are the legacy-markdown fallback: they pull the count off an old
// .md report's Verdict line ("1 leak, 0 watch" → 1; "1 breaking, 1 minor" → 1).
var (
	leakRe     = regexp.MustCompile(`(\d+)\s+leak`)
	breakingRe = regexp.MustCompile(`(\d+)\s+breaking`)
)

// applyFixes converges the prose deterministically: it (1) CUTS every fog LEAK span
// outright — deletion can't re-leak and is monotonic, unlike a model rewrite that
// re-states the sealed thing (the v11 failure: rewrites drove leaks 3→12) — and (2)
// splices the model's continuity rewrites (spanfix.json) in by exact substring. The model
// never sees the whole prose, so it can't lapse into copy-the-document. Spans that no
// longer match verbatim are skipped (the loop's re-check catches any remainder). Returns
// the number of fixes that landed.
func applyFixes(prosePath, spanfixPath, fogReportPath, outPath string) (int, error) {
	prose, err := os.ReadFile(prosePath)
	if err != nil {
		return 0, err
	}
	s := string(prose)
	applied := 0

	// 1. Cut fog LEAK spans.
	if fogRaw, err := os.ReadFile(fogReportPath); err == nil {
		var fog struct {
			Findings []struct {
				Severity string `json:"severity"`
				Span     string `json:"span"`
			} `json:"findings"`
		}
		if json.Unmarshal(world.StripJSONFence(fogRaw), &fog) == nil {
			for _, f := range fog.Findings {
				span := strings.TrimSpace(f.Span)
				if strings.EqualFold(f.Severity, "LEAK") && span != "" {
					if cut, ok := cutSpan(s, span); ok {
						s = cut
						applied++
					}
				}
			}
		}
	}

	// 2. Splice continuity rewrites.
	if raw, err := os.ReadFile(spanfixPath); err == nil {
		var doc struct {
			Replacements []struct {
				Span        string `json:"span"`
				Replacement string `json:"replacement"`
			} `json:"replacements"`
		}
		if json.Unmarshal(world.StripJSONFence(raw), &doc) == nil {
			for _, r := range doc.Replacements {
				span := strings.TrimSpace(r.Span)
				if span == "" || !strings.Contains(s, span) {
					continue
				}
				s = strings.Replace(s, span, r.Replacement, 1)
				applied++
			}
		}
	}

	if err := os.WriteFile(outPath, []byte(s), 0o644); err != nil {
		return 0, err
	}
	return applied, nil
}

// cutSpan removes the first occurrence of span from s and collapses only
// the whitespace run the cut leaves at the splice site — never touching
// intentional doubled spaces elsewhere in the prose (the old code ran a
// document-wide ReplaceAll, silently rewriting the model's deliberate
// spacing). Returns the result and whether the span was found.
func cutSpan(s, span string) (string, bool) {
	i := strings.Index(s, span)
	if i < 0 {
		return s, false
	}
	before := s[:i]
	after := s[i+len(span):]
	// Trim the trailing whitespace run on the left and the leading
	// whitespace run on the right of the seam, then rejoin with a
	// single space when both sides are non-empty and neither boundary
	// is a paragraph break (so we don't glue sentences across blank
	// lines or eat a leading/trailing edge).
	leftTrim := strings.TrimRight(before, " \t")
	rightTrim := strings.TrimLeft(after, " \t")
	switch {
	case leftTrim == "" || rightTrim == "":
		return leftTrim + rightTrim, true
	case strings.HasSuffix(leftTrim, "\n") || strings.HasPrefix(rightTrim, "\n"):
		return leftTrim + rightTrim, true
	default:
		return leftTrim + " " + rightTrim, true
	}
}

// verdictCount returns how many findings of the given severity ("LEAK", "BREAKING", …)
// a check report holds. The checkers now emit JSON ({"findings":[{"severity":…}]}), which
// the model cannot ramble through, so the count is exact. Falls back to the legacy
// markdown Verdict-line regex for old reports. 0 if the file is missing.
func verdictCount(path, severity string) int {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	t := world.StripJSONFence(b)
	if len(t) > 0 && t[0] == '{' {
		var rep struct {
			Findings []struct {
				Severity string `json:"severity"`
				Conflict string `json:"conflict"`
				Issue    string `json:"issue"`
				Fix      string `json:"fix"`
			} `json:"findings"`
		}
		if json.Unmarshal(t, &rep) == nil {
			n := 0
			for _, f := range rep.Findings {
				if !strings.EqualFold(f.Severity, severity) {
					continue
				}
				// Drop findings the checker talked itself out of (self-negating noise).
				reason := f.Conflict
				if reason == "" {
					reason = f.Issue
				}
				if world.IsNonFinding(reason, f.Fix) {
					continue
				}
				n++
			}
			return n
		}
	}
	// Legacy markdown fallback.
	re := leakRe
	if strings.EqualFold(severity, "breaking") {
		re = breakingRe
	}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.Contains(line, "Verdict") {
			continue
		}
		if strings.Contains(line, "CLEAN") {
			return 0
		}
		if m := re.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			return n
		}
		return 0
	}
	return 0
}

// contFinding is one continuity finding as stored in continuity_report.md.
type contFinding struct {
	Severity string `json:"severity"`
	Category string `json:"category,omitempty"`
	Span     string `json:"span"`
	Conflict string `json:"conflict,omitempty"`
	Fix      string `json:"fix,omitempty"`
}

// normForMatch lowercases and collapses all whitespace so a model's "verbatim" quote
// matches the source despite trivial spacing/case differences.
func normForMatch(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// dropPhantomFindings rewrites a JSON findings report (fog or continuity) to drop any
// finding whose `span` does not actually occur in the prose. The 27B checkers read the
// sealed notebook + canon as reference and sometimes QUOTE those sources as if they were
// leaks/contradictions in the installment — observed the fog checker reporting two
// "leaks" that existed only in notebook.md, never in the prose, which the deterministic
// fog-cut then could never remove (the span isn't there to cut). A span not present in
// the prose cannot be a leak or a contradiction IN the prose. Mechanical, no LLM call.
// Returns the number dropped. Best-effort: any parse failure leaves the report
// untouched. prose is the already-normalised (normForMatch) installment text — passed
// in so auditFindings reads + normalises story.md once across both reports. An empty
// prose (e.g. story.md unreadable) means "can't judge presence" → no drop, matching the
// old read-failure behaviour rather than dropping every span as phantom.
func dropPhantomFindings(prose, reportPath string) int {
	if prose == "" {
		return 0
	}
	rb, err := os.ReadFile(reportPath)
	if err != nil {
		return 0
	}
	var rep struct {
		Findings []map[string]any `json:"findings"`
	}
	if json.Unmarshal(world.StripJSONFence(rb), &rep) != nil || len(rep.Findings) == 0 {
		return 0
	}
	kept := make([]map[string]any, 0, len(rep.Findings))
	dropped := 0
	for _, f := range rep.Findings {
		span, _ := f["span"].(string)
		ns := normForMatch(span)
		// Full-span containment only. A byte-prefix fallback (ns[:40]) could
		// split a multibyte rune mid-sequence and miss a present span,
		// dropping a real fog leak as a phantom.
		present := len(ns) < 12 || // too short to judge confidently → keep
			strings.Contains(prose, ns)
		if present {
			kept = append(kept, f)
		} else {
			dropped++
		}
	}
	if dropped == 0 {
		return 0
	}
	if out, err := json.MarshalIndent(struct {
		Findings []map[string]any `json:"findings"`
	}{kept}, "", "  "); err == nil {
		_ = os.WriteFile(reportPath, out, 0o644)
	}
	return dropped
}

// auditFindings cleans both check reports before they are scored: it drops phantom
// (not-in-the-prose) fog + continuity findings, then runs the adversarial verify over the
// remaining continuity findings. After this, verdictCount reflects real, in-prose,
// canon-backed findings — the signal the verify-loop and best-of-N optimise against.
func auditFindings(cmd *cobra.Command, layout world.Layout, n int, installmentDir, prosePath, fogRpt, contRpt, worldFile, canonFile string) {
	// Read + normalise the prose once; both phantom-drops match spans against it.
	var prose string
	if pb, err := os.ReadFile(prosePath); err == nil {
		prose = normForMatch(string(pb))
	}
	if d := dropPhantomFindings(prose, fogRpt); d > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  fog-audit: dropped %d phantom leak(s) not present in the prose\n", d)
	}
	if d := dropPhantomFindings(prose, contRpt); d > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  continuity-audit: dropped %d phantom finding(s) not present in the prose\n", d)
	}
	_ = verifyContinuity(cmd, layout, n, installmentDir, contRpt, worldFile, canonFile)
}

// verifyContinuity runs the adversarial false-positive audit over the current
// continuity_report.md and rewrites it in place to keep ONLY findings the audit confirms
// REAL with a contradicted canon line that ACTUALLY occurs in the bible/canon. This turns
// the noisy 27B continuity checker (which over-flags absence-is-not-prohibition cases and
// even emits self-negating non-findings) into a score that tracks real breaks — which is
// what makes the verify-loop and best-of-N optimise against something true. Best-effort:
// any parse/run failure leaves the report untouched (we never silently drop unaudited work).
func verifyContinuity(cmd *cobra.Command, layout world.Layout, n int, installmentDir, contRptPath, worldFile, canonFile string) error {
	raw, err := os.ReadFile(contRptPath)
	if err != nil {
		return nil // no report yet — nothing to verify
	}
	var rep struct {
		Findings []contFinding `json:"findings"`
	}
	if json.Unmarshal(world.StripJSONFence(raw), &rep) != nil || len(rep.Findings) == 0 {
		return nil // unparseable or empty — leave as-is
	}

	if err := runPipeline(cmd, installmentDir, func() (*vamp.Pipeline, error) {
		return pipeline.BuildContinuityVerify(pipeline.ContinuityVerifyConfig{
			ContinuityReportFile: contRptPath,
			WorldFile:            worldFile,
			CanonFile:            canonFile,
			OutputName:           "continuity_verify.json",
		})
	}); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "  continuity-verify skipped (%v) — keeping unaudited findings\n", err)
		return nil
	}

	vraw, err := os.ReadFile(layout.InstallmentFile(n, "continuity_verify.json"))
	if err != nil {
		return nil
	}
	var vrep struct {
		Verdicts []struct {
			Span       string `json:"span"`
			Verdict    string `json:"verdict"`
			CanonQuote string `json:"canon_quote"`
		} `json:"verdicts"`
	}
	if json.Unmarshal(world.StripJSONFence(vraw), &vrep) != nil {
		return nil // bad audit output — keep findings unaudited
	}

	// Corpus the cited quote must actually appear in (so the model can't fabricate one).
	// A REAL verdict is only kept when its quote occurs here, so an unreadable corpus
	// would fail every quote check and drop even genuine findings. Bail when both reads
	// fail (or the corpus is empty) and leave the findings untouched.
	corpus := ""
	bw, ew := os.ReadFile(worldFile)
	if ew == nil {
		corpus += normForMatch(string(bw))
	}
	bc, ec := os.ReadFile(canonFile)
	if ec == nil {
		corpus += " " + normForMatch(string(bc))
	}
	if (ew != nil && ec != nil) || strings.TrimSpace(corpus) == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "  continuity-verify skipped (could not read world.md/canon.md) — keeping unaudited findings")
		return nil
	}
	verdictBySpan := map[string]struct {
		verdict string
		quote   string
	}{}
	for _, v := range vrep.Verdicts {
		verdictBySpan[normForMatch(v.Span)] = struct {
			verdict string
			quote   string
		}{v.Verdict, v.CanonQuote}
	}

	kept := make([]contFinding, 0, len(rep.Findings))
	dropped := 0
	for _, f := range rep.Findings {
		v, ok := verdictBySpan[normForMatch(f.Span)]
		if !ok {
			kept = append(kept, f) // unaudited — keep, don't silently drop
			continue
		}
		q := normForMatch(v.quote)
		// REAL requires a non-trivial cited quote that genuinely occurs in the bible/canon.
		if strings.EqualFold(v.verdict, "REAL") && len(q) >= 12 && strings.Contains(corpus, q) {
			kept = append(kept, f)
		} else {
			dropped++
		}
	}
	if dropped == 0 {
		return nil // nothing to rewrite
	}
	out, err := json.MarshalIndent(struct {
		Findings []contFinding `json:"findings"`
	}{kept}, "", "  ")
	if err != nil {
		return nil
	}
	if err := os.WriteFile(contRptPath, out, 0o644); err != nil {
		return fmt.Errorf("rewrite verified continuity report: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  continuity-verify: dropped %d false positive(s), %d real finding(s) remain\n", dropped, len(kept))
	return nil
}

// generatePerSceneDraft runs the outline, then writes each scene as its own LLM pass
// (sequential — each scene sees the prose of the scenes already written) and stitches
// them into one draft. Returns the stitched-draft path and the outline JSON. This is
// the length mechanism: per-scene word budgets the model honours, in place of a single
// pass whose length cannot be steered.
func generatePerSceneDraft(cmd *cobra.Command, layout world.Layout, n int, cfg pipeline.StoryConfig, installmentDir string) (draftFile, outlineJSON string, err error) {
	out := cmd.OutOrStdout()

	// 1. Outline.
	outDir := filepath.Join(installmentDir, "outline")
	if err = runPipeline(cmd, outDir, func() (*vamp.Pipeline, error) {
		return pipeline.BuildOutline(pipeline.OutlineConfig{
			WorldFile:             cfg.WorldFile,
			CharactersFile:        cfg.CharactersFile,
			CanonFile:             cfg.CanonFile,
			CanonRelevantFile:     cfg.CanonRelevantFile,
			PriorsFile:            cfg.PriorsFile,
			BriefFile:             cfg.BriefFile,
			HistoricalContextFile: cfg.HistoricalContextFile,
			NotebookFile:          cfg.NotebookFile,
			TargetWords:           cfg.TargetWords,
		})
	}); err != nil {
		return "", "", fmt.Errorf("outline: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(outDir, "outline.json"))
	if err != nil {
		return "", "", fmt.Errorf("read outline: %w", err)
	}
	outlineJSON = string(raw)
	outlinePath := filepath.Join(installmentDir, "outline.json")
	if werr := os.WriteFile(outlinePath, raw, 0o644); werr != nil {
		return "", "", fmt.Errorf("persist outline: %w", werr)
	}

	// 2. Parse the scenes (raw objects — no need to model every field; write_scene
	// renders them).
	var plan struct {
		Scenes []json.RawMessage `json:"scenes"`
	}
	if err = json.Unmarshal(raw, &plan); err != nil {
		return "", "", fmt.Errorf("parse outline scenes: %w", err)
	}
	if len(plan.Scenes) == 0 {
		return "", "", fmt.Errorf("outline produced no scenes")
	}
	fmt.Fprintf(out, "per-scene authoring: %d scenes\n", len(plan.Scenes))

	// 3. Sequential per-scene generation (each scene sees the prior prose).
	scenesDir := filepath.Join(installmentDir, "scenes")
	if err = os.MkdirAll(scenesDir, 0o755); err != nil {
		return "", "", err
	}
	var stitched bytes.Buffer
	for i, sc := range plan.Scenes {
		idx := i + 1
		sceneDir := filepath.Join(scenesDir, fmt.Sprintf("%03d", idx))
		if err = os.MkdirAll(sceneDir, 0o755); err != nil {
			return "", "", err
		}
		specPath := filepath.Join(sceneDir, "spec.json")
		if err = os.WriteFile(specPath, sc, 0o644); err != nil {
			return "", "", err
		}
		priorPath := filepath.Join(sceneDir, "prior.md")
		if err = os.WriteFile(priorPath, stitched.Bytes(), 0o644); err != nil {
			return "", "", err
		}

		fmt.Fprintf(out, "  scene %d/%d...\n", idx, len(plan.Scenes))
		scfg := pipeline.SceneProseConfig{
			WorldFile:             cfg.WorldFile,
			CharactersFile:        cfg.CharactersFile,
			CanonRelevantFile:     cfg.CanonRelevantFile,
			PriorsFile:            cfg.PriorsFile,
			BriefFile:             cfg.BriefFile,
			HistoricalContextFile: cfg.HistoricalContextFile,
			NotebookFile:          cfg.NotebookFile,
			LicensedRevealsFile:   cfg.LicensedRevealsFile,
			ChapterFactsFile:      cfg.ChapterFactsFile,
			OutlineFile:           outlinePath,
			SceneSpecFile:         specPath,
			PriorProseFile:        priorPath,
			SceneIndex:            idx,
			SceneCount:            len(plan.Scenes),
		}
		// best-of-N: give each scene a distinct seed derived from the attempt's base
		// seed, so attempt #2 samples different prose than #1 (temp 0.8) rather than
		// replaying the cache. Unseeded (cfg.Seed==0) leaves the writer untouched.
		if cfg.Seed != 0 {
			scfg.Seed = cfg.Seed*1000 + idx
		}
		if err = runPipeline(cmd, sceneDir, func() (*vamp.Pipeline, error) {
			return pipeline.BuildSceneProse(scfg)
		}); err != nil {
			return "", "", fmt.Errorf("scene %d: %w", idx, err)
		}
		sb, rerr := os.ReadFile(filepath.Join(sceneDir, fmt.Sprintf("scene_%03d.md", idx)))
		if rerr != nil {
			return "", "", fmt.Errorf("read scene %d: %w", idx, rerr)
		}
		if stitched.Len() > 0 {
			stitched.WriteString("\n\n")
		}
		stitched.Write(bytes.TrimSpace(sb))
	}

	// 4. Stitch.
	draftFile = filepath.Join(installmentDir, "stitched_draft.md")
	if err = os.WriteFile(draftFile, append(stitched.Bytes(), '\n'), 0o644); err != nil {
		return "", "", fmt.Errorf("write stitched draft: %w", err)
	}
	fmt.Fprintf(out, "per-scene draft: %d words across %d scenes\n",
		len(strings.Fields(stitched.String())), len(plan.Scenes))
	return draftFile, outlineJSON, nil
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

// publishChapters copies every finished per-chapter episode.m4b into
// publishTo. Used as the fallback when book assembly fails (e.g. ffmpeg
// is absent) but the user asked to publish — better to deliver the
// chapters than to exit zero with nothing copied. Returns how many were
// published.
func publishChapters(cmd *cobra.Command, layout world.Layout, count int, publishTo string) (int, error) {
	published := 0
	for i := 1; i <= count; i++ {
		if _, err := os.Stat(layout.InstallmentFile(i, "episode.m4b")); err != nil {
			continue
		}
		if err := publishEpisode(cmd, layout, i, publishTo); err != nil {
			return published, err
		}
		published++
	}
	return published, nil
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
			return root.ExecuteContext(cmd.Context())
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
			return root.ExecuteContext(cmd.Context())
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

// lockWorld takes the per-world exclusive lock for a mutating command and
// returns a release func to defer. Two concurrent mutating runs on the same
// world (story, expand, timeline edits, …) would corrupt shared state
// (canon.md, timeline.json, the run dirs), so every mutating RunE grabs this
// first; read-only commands (list, score, ask, codex, timeline list/show)
// don't. On contention it returns a clear "locked by another process" error.
func lockWorld(l world.Layout) (func(), error) {
	lk, err := world.Acquire(l)
	if err != nil {
		return nil, err
	}
	return func() { _ = lk.Unlock() }, nil
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
// validateNarrator rejects an unknown --narrator voice BEFORE the expensive prose
// pipeline runs. Unlike the per-shot voice path (which silently normalizes a
// machine-generated voice_id), a user-typed narrator typo should fail fast and
// loudly — otherwise the whole installment generates and only 400s at the TTS stage,
// after all the LLM work is done.
func validateNarrator(voice string) error {
	if world.ValidVoice(voice) {
		return nil
	}
	return fmt.Errorf("unknown narrator voice %q; valid voices: %s",
		voice, strings.Join(world.KnownVoices(), ", "))
}

// copyFile is the cmd-side alias for world.CopyFile, kept so the many call sites read
// naturally. The single streaming implementation lives in the world package.
func copyFile(src, dst string) error { return world.CopyFile(src, dst) }
