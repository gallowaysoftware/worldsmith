package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/vibe/vamp"

	"github.com/gallowaysoftware/worldsmith/internal/pipeline"
	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// benchCommand A/Bs candidate `long_form` profiles on a fixed
// installment, producing blind-labelled prose outputs for human
// scoring.
//
// Why blind: a reader who knows which model produced which output
// will involuntarily favour the model they expect to win. The
// command writes per-candidate output to `bench/<n>/A/`, `B/`...
// and saves the cipher to a separate file at `bench/<n>/cipher.json`
// that the user is expected NOT to peek at until after scoring.
//
// Capabilities mutation: the only knob vamp exposes for "which
// profile satisfies long_form" lives in capabilities.yaml. The
// command saves the original file, edits long_form's candidate list
// to `[<this candidate>, fast]`, runs, and restores on exit (deferred,
// so an interrupted bench still restores). The `fast` fallback
// stays in place so a VRAM oversubscribe doesn't fail the whole
// bench — instead the run lands on the smaller model and the
// resulting story.md makes the degradation obvious.
func benchCommand() *cobra.Command {
	var (
		slug         string
		installment  int
		candidatesIn []string
	)
	cmd := &cobra.Command{
		Use:   "bench",
		Short: "A/B candidate long_form profiles on a fixed installment; outputs blinded for human scoring.",
		Long: `bench runs the prose-only stages of the story pipeline (write_story +
edit_story) once per candidate profile, with capabilities.yaml
temporarily rewritten so each run uses that candidate. Per-candidate
outputs land in bench/<installment>/<letter>/story.md with a sealed
cipher.json mapping letter → profile so the user can score blind
and reveal afterwards.

The rubric ships next to the outputs as rubric.md — read both
outputs, fill in each axis per candidate, THEN open cipher.json.
Don't peek.

Candidates are vibe profile names; pass at least two
(--candidates long_form,bench_qwen3_32b).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(candidatesIn) < 2 {
				return fmt.Errorf("--candidates requires at least two profile names")
			}
			candidates := uniq(candidatesIn)
			if len(candidates) < 2 {
				return fmt.Errorf("--candidates must contain at least two DISTINCT profile names")
			}

			layout, err := world.Open(slug)
			if err != nil {
				return err
			}
			if _, err := os.Stat(layout.WorldFile()); err != nil {
				return fmt.Errorf("world.md not found at %s", layout.WorldFile())
			}
			if _, err := os.Stat(layout.BriefFile(installment)); err != nil {
				return fmt.Errorf("brief not found at %s — write one before benching", layout.BriefFile(installment))
			}

			// Compute the inputs the same way runStory does:
			// brief frontmatter → timeline filter → historical_context.md.
			brief, _, err := world.ParseBrief(layout.BriefFile(installment))
			if err != nil {
				return fmt.Errorf("parse brief: %w", err)
			}
			t, err := world.LoadTimeline(layout)
			if err != nil {
				return fmt.Errorf("load timeline: %w", err)
			}
			filterOpts := world.FilterOptsFromBrief(brief, t.Calendar)
			canonPath, err := world.EnsureCanonFile(layout)
			if err != nil {
				return err
			}

			benchRoot := filepath.Join(layout.Root, "bench", fmt.Sprintf("%03d", installment))
			if err := os.MkdirAll(benchRoot, 0o755); err != nil {
				return err
			}

			capsPath := capabilitiesPath()
			restoreCaps, err := snapshotCapabilities(capsPath)
			if err != nil {
				return fmt.Errorf("snapshot capabilities.yaml: %w", err)
			}
			defer restoreCaps()

			// Stable letter assignment — A, B, C... in the order
			// the user typed them. The cipher rotates them later.
			results := make([]benchResult, 0, len(candidates))
			for i, profile := range candidates {
				letter := string('A' + rune(i))
				outDir := filepath.Join(benchRoot, "raw_"+profile)
				if err := os.MkdirAll(outDir, 0o755); err != nil {
					return err
				}
				priorsPath, err := world.EnsurePriorsFile(layout, outDir, installment)
				if err != nil {
					return fmt.Errorf("priors: %w", err)
				}
				histPath, err := world.WriteHistoricalContext(outDir, t.Events, filterOpts)
				if err != nil {
					return fmt.Errorf("historical context: %w", err)
				}

				fmt.Fprintf(cmd.OutOrStdout(),
					"\n=== candidate [%s] %s ===\n", letter, profile)

				if err := writeCapabilitiesForCandidate(capsPath, profile); err != nil {
					return err
				}

				cfg := pipeline.BenchStoryConfig{
					WorldFile:             layout.WorldFile(),
					CharactersFile:        layout.CharactersFile(),
					CanonFile:             canonPath,
					PriorsFile:            priorsPath,
					BriefFile:             layout.BriefFile(installment),
					HistoricalContextFile: histPath,
				}

				root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
					return pipeline.BuildBenchStory(cfg)
				})
				if err != nil {
					return err
				}
				start := time.Now()
				root.SetArgs([]string{"run", "--run-dir", outDir, "--no-cache"})
				runErr := root.ExecuteContext(cmd.Context())
				dur := time.Since(start)

				story := filepath.Join(outDir, "story.md")
				if _, err := os.Stat(story); err != nil {
					if runErr != nil {
						return fmt.Errorf("candidate %s failed: %w", profile, runErr)
					}
					return fmt.Errorf("%w: %s", pipeline.ErrBenchOutputMissing, profile)
				}
				results = append(results, benchResult{
					Letter:   letter,
					Profile:  profile,
					Duration: dur,
					RawDir:   outDir,
					StoryMD:  story,
				})
				fmt.Fprintf(cmd.OutOrStdout(), "  wrote %s (%s)\n", story, formatDuration(dur))
			}

			// Blind shuffle: re-letter the results so the human can't
			// guess by file order which was first.
			shuffleResults(results)
			for i := range results {
				results[i].BlindLetter = string('A' + rune(i))
				blindDir := filepath.Join(benchRoot, results[i].BlindLetter)
				if err := os.MkdirAll(blindDir, 0o755); err != nil {
					return err
				}
				blindStory := filepath.Join(blindDir, "story.md")
				if err := copyFile(results[i].StoryMD, blindStory); err != nil {
					return fmt.Errorf("copy to blind: %w", err)
				}
			}

			// Write cipher + rubric.
			cipher := make([]benchCipherEntry, 0, len(results))
			for _, r := range results {
				cipher = append(cipher, benchCipherEntry{
					BlindLetter: r.BlindLetter,
					Profile:     r.Profile,
					DurationMS:  r.Duration.Milliseconds(),
				})
			}
			if err := writeCipher(filepath.Join(benchRoot, "cipher.json"), cipher); err != nil {
				return fmt.Errorf("write cipher: %w", err)
			}
			if err := writeRubric(filepath.Join(benchRoot, "rubric.md"), results); err != nil {
				return fmt.Errorf("write rubric: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"\nbench complete. read:\n")
			for _, r := range results {
				fmt.Fprintf(cmd.OutOrStdout(),
					"  %s\n", filepath.Join(benchRoot, r.BlindLetter, "story.md"))
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"\nscore each in %s, then `cat %s` to reveal.\n",
				filepath.Join(benchRoot, "rubric.md"),
				filepath.Join(benchRoot, "cipher.json"))
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (required).")
	cmd.Flags().IntVar(&installment, "installment", 1, "Installment number whose brief drives the bench.")
	cmd.Flags().StringSliceVar(&candidatesIn, "candidates", nil,
		"Comma-separated list of vibe profile names to A/B (at least 2 distinct).")
	_ = cmd.MarkFlagRequired("slug")
	_ = cmd.MarkFlagRequired("candidates")
	return cmd
}

type benchResult struct {
	Letter      string
	BlindLetter string
	Profile     string
	Duration    time.Duration
	RawDir      string
	StoryMD     string
}

type benchCipherEntry struct {
	BlindLetter string `json:"blind_letter"`
	Profile     string `json:"profile"`
	DurationMS  int64  `json:"duration_ms"`
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// shuffleResults Fisher-Yates shuffles in place. crypto/rand source
// so the cipher can't be predicted from the slug + a known seed.
func shuffleResults(rs []benchResult) {
	for i := len(rs) - 1; i > 0; i-- {
		j := cryptoIntn(i + 1)
		rs[i], rs[j] = rs[j], rs[i]
	}
}

func cryptoIntn(n int) int {
	if n <= 0 {
		return 0
	}
	max := big.NewInt(int64(n))
	v, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

// capabilitiesPath returns the path to vamp's capabilities.yaml,
// honouring XDG_CONFIG_HOME, falling back to ~/.config/vamp/.
func capabilitiesPath() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "vamp", "capabilities.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vamp", "capabilities.yaml")
}

// snapshotCapabilities reads the current capabilities.yaml into a
// backup buffer + returns a closure that restores it. The defer on
// the bench command's RunE invokes this so even Ctrl-C / partial
// failure leaves the file unchanged.
func snapshotCapabilities(path string) (func(), error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return func() {
		_ = os.WriteFile(path, raw, 0o644)
	}, nil
}

// writeCapabilitiesForCandidate rewrites capabilities.yaml's long_form
// candidate list to point at `candidate`. Preserves every other line
// of the file verbatim — only the `candidates: [...]` value under
// the long_form key is replaced.
//
// Implementation: find the indented `candidates:` line under
// long_form (string-search; capabilities.yaml is hand-edited so
// preserving comments + whitespace matters more than YAML round-trip
// purity).
func writeCapabilitiesForCandidate(path, candidate string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	inLongForm := false
	rewrote := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		switch {
		case trim == "long_form:":
			inLongForm = true
		case inLongForm && strings.HasPrefix(trim, "candidates:"):
			// Preserve indentation.
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = fmt.Sprintf("%scandidates: [%s, fast]", indent, candidate)
			inLongForm = false
			rewrote = true
		case strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t"):
			// still inside the long_form block — pass
		default:
			if trim != "" && !strings.HasPrefix(trim, "#") {
				inLongForm = false
			}
		}
	}
	if !rewrote {
		return fmt.Errorf("capabilities.yaml: didn't find a long_form.candidates line to rewrite")
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func writeCipher(path string, entries []benchCipherEntry) error {
	raw, err := json.MarshalIndent(map[string]any{"cipher": entries}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func writeRubric(path string, results []benchResult) error {
	var b strings.Builder
	b.WriteString("# Bench rubric\n\n")
	b.WriteString("Score each candidate on each axis from 1-5. Read the candidate's `story.md` first; don't open `cipher.json` until you've finished both columns.\n\n")
	b.WriteString("| Axis | What to look for |\n")
	b.WriteString("|---|---|\n")
	b.WriteString("| Voice fidelity | Matches `world.md`'s declared tone (Le Guin-quiet vs Sanderson-busy, etc.). |\n")
	b.WriteString("| Character consistency | Named cast acts in character per `characters.json`. Specific tics + voice mannerisms preserved. |\n")
	b.WriteString("| Restraint | Subtext stays subtext. The model that *explains* what could be inferred is failing. |\n")
	b.WriteString("| Specificity | Concrete sensory detail per page. \"Slate-grey water\" vs \"the sea.\" |\n")
	b.WriteString("| Earned pacing | The brief's stated scene length feels real, not padded. |\n")
	b.WriteString("| Tic density | Mechanical sniff for LLM tics (`she felt`, `couldn't shake`, em-dash overuse, generic similes). |\n\n")
	b.WriteString("## Scores\n\n")
	b.WriteString("| Axis | ")
	for _, r := range results {
		fmt.Fprintf(&b, "%s | ", r.BlindLetter)
	}
	b.WriteString("\n|---|")
	for range results {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, axis := range []string{"Voice fidelity", "Character consistency", "Restraint", "Specificity", "Earned pacing", "Tic density (lower = better)"} {
		fmt.Fprintf(&b, "| %s |", axis)
		for range results {
			b.WriteString("  |")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n## Notes\n\n")
	for _, r := range results {
		fmt.Fprintf(&b, "### %s\n\n_(write your impressions here)_\n\n", r.BlindLetter)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func formatDuration(d time.Duration) string {
	if d >= time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(100 * time.Millisecond).String()
}
