package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/vibe/contentkit"

	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// scoreCommand shows the per-installment quality scorecards for a world and the
// trend across them — measurement, not vibes (goal #4). Scores are recomputed
// from each installment's story.md + continuity_report.md, so it works on
// installments generated before scorecards existed.
func scoreCommand() *cobra.Command {
	var slug string
	cmd := &cobra.Command{
		Use:   "score <slug>",
		Short: "Show per-installment quality scorecards + the trend.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required")
			}
			return runScore(cmd, slug)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	return cmd
}

func runScore(cmd *cobra.Command, slug string) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	done, err := world.CompletedInstallments(layout)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(done) == 0 {
		fmt.Fprintf(out, "no finished installments for %s yet\n", slug)
		return nil
	}

	fmt.Fprintf(out, "%s — quality scorecards\n", slug)
	fmt.Fprintf(out, "%-5s %-9s %-44s %-26s %s\n", "inst", "overall", "prose", "continuity", "fog")
	var overalls []int
	for _, n := range done {
		card := world.BuildScorecard(layout, n)
		ov := world.Overall(card)
		overalls = append(overalls, ov)
		prose := resultByName(card, "prose")
		cont := resultByName(card, "continuity")
		fog := resultByName(card, "fog")
		fmt.Fprintf(out, "%03d   %3d/100   %-44s %-26s %s\n", n, ov, trimSummary(prose), trimSummary(cont), fogCol(fog))
	}

	// Trend: compare the latest to the mean of the rest.
	if len(overalls) >= 2 {
		latest := overalls[len(overalls)-1]
		sum := 0
		for _, v := range overalls[:len(overalls)-1] {
			sum += v
		}
		prevMean := sum / (len(overalls) - 1)
		var arrow string
		switch {
		case latest > prevMean+3:
			arrow = "↑ improving"
		case latest < prevMean-3:
			arrow = "↓ slipping"
		default:
			arrow = "→ steady"
		}
		fmt.Fprintf(out, "\ntrend: latest %d vs prior-mean %d  %s\n", latest, prevMean, arrow)
	}
	return nil
}

func resultByName(card contentkit.Scorecard, name string) contentkit.ScoreResult {
	for _, r := range card.Results {
		if len(r.Summary) >= len(name) && r.Summary[:len(name)] == name {
			return r
		}
	}
	return contentkit.ScoreResult{}
}

// fogCol renders the fog axis, or an em-dash when the installment predates
// fog-checking (no fog_report.md, so the axis is absent from the card).
func fogCol(r contentkit.ScoreResult) string {
	if r.Summary == "" {
		return "—"
	}
	return trimSummary(r)
}

func trimSummary(r contentkit.ScoreResult) string {
	s := r.Summary
	// drop the "<name>: " prefix for compact display
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return fmt.Sprintf("%3d %s", r.Score, s[i+2:])
		}
	}
	return fmt.Sprintf("%3d %s", r.Score, s)
}
