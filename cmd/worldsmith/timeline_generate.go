package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/vibe/vamp"

	"github.com/gallowaysoftware/worldsmith/internal/pipeline"
	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// timelineGenerateCommand runs the five-pass LLM timeline generator
// against the world's bible + characters and appends the result to
// timeline.json as proposed events. The human then runs
// `worldsmith timeline review --slug <slug>` to promote keepers to
// canon.
//
// The pipeline run dir lands under ~/.local/state/worldsmith/<slug>/timeline-gen/<ts>/
// so multiple gen runs accumulate side-by-side and the user can
// inspect the per-pass JSON output if they want to debug a bad
// proposal.
func timelineGenerateCommand() *cobra.Command {
	var (
		slug      string
		runDir    string
		autoMerge bool
	)
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Run the five-pass LLM timeline generator and append proposed events.",
		Long: `generate runs the five-pass timeline-gen pipeline:

  seed_eras          → 3-7 named eras
  seed_anchors       → high-scope events anchoring each era
  elaborate_regional → regional / local consequences
  personalise        → personal-scale events tying named characters in
  fog_pass           → visibility tiers (common/regional/cloistered/secret/lost)

Output lands as proposed events in the world's timeline.json (run
` + "`worldsmith timeline review`" + ` to promote keepers to canon).
The full per-pass JSON is preserved in the run dir for debugging.

Requires the long_form vibe profile to be activatable (the auto-
ensure-services preflight brings it up when this command runs).
~5-10 minutes wall-clock on Qwen3.6-27B; longer with CoT on.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			layout, err := world.Open(slug)
			if err != nil {
				return err
			}
			if _, err := os.Stat(layout.WorldFile()); err != nil {
				return fmt.Errorf("world.md not found at %s — run `worldsmith init %s` first",
					layout.WorldFile(), slug)
			}

			if runDir == "" {
				runDir = filepath.Join(layout.Root, "timeline-gen",
					time.Now().Local().Format("2006-01-02T15-04-05"))
			}
			if err := os.MkdirAll(runDir, 0o755); err != nil {
				return err
			}

			// Pre-existing timeline (when present) is fed back into
			// the prompts so the generator doesn't contradict
			// already-canon events. Path may be empty when no
			// timeline.json exists yet — the loader handles that
			// silently inside readFileOrEmpty.
			existing := ""
			if _, err := os.Stat(layout.TimelineFile()); err == nil {
				existing = layout.TimelineFile()
			}

			cfg := pipeline.TimelineGenConfig{
				WorldFile:            layout.WorldFile(),
				CharactersFile:       layout.CharactersFile(),
				ExistingTimelineFile: existing,
			}

			fmt.Fprintf(cmd.OutOrStdout(), "world:   %s\n", slug)
			fmt.Fprintf(cmd.OutOrStdout(), "run dir: %s\n\n", runDir)

			root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
				return pipeline.BuildTimelineGen(cfg)
			})
			if err != nil {
				return err
			}
			root.SetArgs([]string{"run", "--run-dir", runDir})
			if err := root.Execute(); err != nil {
				return fmt.Errorf("timeline-gen: %w", err)
			}

			// Merge per-pass JSON into a single proposed-events
			// list + update the world's timeline.json.
			eras, events, err := world.MergeGeneratedTimeline(runDir)
			if err != nil {
				return fmt.Errorf("merge generated outputs: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\ngenerated: %d era(s), %d event(s)\n",
				len(eras), len(events))

			if !autoMerge {
				fmt.Fprintf(cmd.OutOrStdout(),
					"\nproposed events are in:\n  %s\n\nrun `worldsmith timeline review --slug %s` to walk through them.\n",
					filepath.Join(runDir, "(anchors|regional|personal|visibilities).json"), slug)
			}

			// Even without --auto-merge, we always persist the
			// proposed events into the world's timeline.json so
			// `timeline list --proposed` shows them. The review
			// step is what promotes proposed→canon.
			t, err := world.LoadTimeline(layout)
			if err != nil {
				return err
			}
			// Merge eras into Calendar.EraAnchors (additive, no
			// overwrite of existing).
			existingEra := make(map[string]bool, len(t.Calendar.EraAnchors))
			for _, e := range t.Calendar.EraAnchors {
				existingEra[e.Slug] = true
			}
			for _, e := range eras {
				if !existingEra[e.Slug] {
					t.Calendar.EraAnchors = append(t.Calendar.EraAnchors, e)
				}
			}
			if err := world.SaveTimeline(layout, t); err != nil {
				return fmt.Errorf("save calendar: %w", err)
			}
			added, err := world.AppendProposedEvents(layout, events)
			if err != nil {
				return fmt.Errorf("append proposed: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "appended %d new event(s) to %s as proposed.\n",
				added, layout.TimelineFile())
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (required).")
	cmd.Flags().StringVar(&runDir, "run-dir", "", "Override the per-run scratch dir. Default: $XDG_STATE_HOME/worldsmith/<slug>/timeline-gen/<ts>.")
	cmd.Flags().BoolVar(&autoMerge, "auto-merge", false, "Reserved: currently a no-op (proposed events always land in timeline.json; canon promotion stays manual).")
	// Hidden until it does something: a visible --auto-merge implies it
	// auto-promotes to canon, which it does not.
	_ = cmd.Flags().MarkHidden("auto-merge")
	_ = cmd.MarkFlagRequired("slug")
	return cmd
}
