package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// timelineCommand returns the `worldsmith timeline` umbrella with
// list/show/add/review subcommands. The fifth subcommand,
// `timeline generate`, lives in timeline_generate.go and is registered
// below.
func timelineCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "Manage the world's historical timeline (events + visibility).",
		Long: `timeline reads + writes timeline.json — the world's record of
historical events with fog-of-war visibility. Subcommands:

  list     show events (canon by default; --proposed to see LLM-generated drafts)
  show     dump one event's full record
  add      append a hand-authored event (interactive prompts)
  review   walk through proposed events, accept/edit/reject each

Storage: a flat timeline.json at the world root by default. Worlds
that grow past ~500 events can split into per-era files under
timeline/<era>.json — the loader concatenates them automatically.`,
	}
	cmd.AddCommand(timelineListCommand())
	cmd.AddCommand(timelineShowCommand())
	cmd.AddCommand(timelineAddCommand())
	cmd.AddCommand(timelineReviewCommand())
	cmd.AddCommand(timelineGenerateCommand())
	return cmd
}

func timelineListCommand() *cobra.Command {
	var (
		slug     string
		proposed bool
		all      bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events in the world's timeline.",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := world.Open(slug)
			if err != nil {
				return err
			}
			t, err := world.LoadTimeline(l)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "calendar: epoch=%q current_year=%d\n", t.Calendar.EpochLabel, t.Calendar.CurrentYear)
			if len(t.Events) == 0 {
				fmt.Fprintf(out, "no events yet — `worldsmith timeline add --slug %s` to author one, or `worldsmith timeline generate --slug %s` (when the GPU is free).\n", slug, slug)
				return nil
			}
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "year\tid\tkind\ttier\tconf\tsource\tsummary")
			filtered := t.Events
			if !all {
				filtered = filtered[:0]
				for _, e := range t.Events {
					switch {
					case proposed && e.Confidence == world.ConfidenceProposed:
						filtered = append(filtered, e)
					case !proposed && (e.Confidence == world.ConfidenceCanon || e.Confidence == ""):
						filtered = append(filtered, e)
					}
				}
			}
			sort.SliceStable(filtered, func(i, j int) bool {
				if filtered[i].Year != filtered[j].Year {
					return filtered[i].Year < filtered[j].Year
				}
				return filtered[i].ID < filtered[j].ID
			})
			for _, e := range filtered {
				tier := e.Visibility.Tier
				if tier == "" {
					tier = world.TierCommon
				}
				conf := e.Confidence
				if conf == "" {
					conf = world.ConfidenceCanon
				}
				src := e.Source
				if src == "" {
					src = "human"
				}
				summary := strings.TrimSpace(e.Summary)
				if len(summary) > 80 {
					summary = summary[:77] + "..."
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n",
					e.Year, e.ID, e.Kind, tier, conf, src, summary)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (required).")
	cmd.Flags().BoolVar(&proposed, "proposed", false, "Show only proposed (LLM-generated, awaiting review) events instead of canon.")
	cmd.Flags().BoolVar(&all, "all", false, "Show every event regardless of confidence (overrides --proposed).")
	_ = cmd.MarkFlagRequired("slug")
	return cmd
}

func timelineShowCommand() *cobra.Command {
	var slug string
	cmd := &cobra.Command{
		Use:   "show <event-id>",
		Short: "Pretty-print one event's full record (including visibility envelope).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := world.Open(slug)
			if err != nil {
				return err
			}
			t, err := world.LoadTimeline(l)
			if err != nil {
				return err
			}
			for _, e := range t.Events {
				if e.ID == args[0] {
					raw, err := json.MarshalIndent(e, "", "  ")
					if err != nil {
						return err
					}
					fmt.Fprintln(cmd.OutOrStdout(), string(raw))
					return nil
				}
			}
			return fmt.Errorf("event %q not found", args[0])
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (required).")
	_ = cmd.MarkFlagRequired("slug")
	return cmd
}

func timelineAddCommand() *cobra.Command {
	var (
		slug    string
		year    int
		kind    string
		scope   string
		region  string
		summary string
		tier    string
		actors  []string
		known   []string
		rumour  string
		tags    []string
		id      string
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Append a hand-authored event to the world's timeline.",
		Long: `add appends one event with source=human + confidence=canon. The
event ID is auto-generated (evt_NNNN, next available) when --id is
omitted. Provide --kind and --summary (prompted on stdin if omitted);
--year defaults to the calendar's current year. Most flags can also be
left out and answered via stdin if you prefer interactive prompts.

Tier defaults to "common". For regional events provide --region. For
cloistered events provide --known-to (repeatable). For both, --rumour
sets the publicly-told distortion that non-knowers see.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := world.Open(slug)
			if err != nil {
				return err
			}
			t, err := world.LoadTimeline(l)
			if err != nil {
				return err
			}

			interactive := summary == "" || kind == ""
			if interactive {
				rdr := bufio.NewReader(os.Stdin)
				if kind == "" {
					kind = prompt(rdr, cmd.OutOrStdout(),
						"kind (war|founding|death|birth|treaty|discovery|...) > ")
				}
				if summary == "" {
					summary = prompt(rdr, cmd.OutOrStdout(),
						"summary (one sentence) > ")
				}
				if year == 0 {
					yearStr := prompt(rdr, cmd.OutOrStdout(),
						fmt.Sprintf("year [default current=%d] > ", t.Calendar.CurrentYear))
					if yearStr == "" {
						year = t.Calendar.CurrentYear
					} else {
						y, err := strconv.Atoi(yearStr)
						if err != nil {
							return fmt.Errorf("year must be an integer: %w", err)
						}
						year = y
					}
				}
			}
			if year == 0 {
				year = t.Calendar.CurrentYear
			}
			if kind == "" || summary == "" {
				return fmt.Errorf("--kind and --summary are required")
			}
			if tier == "" {
				tier = world.TierCommon
			}
			if scope == "" {
				scope = "local"
			}

			if id == "" {
				id = nextEventID(t.Events)
			}

			e := world.Event{
				ID:      id,
				Year:    year,
				Kind:    kind,
				Scope:   scope,
				Region:  region,
				Actors:  actors,
				Summary: summary,
				Tags:    tags,
				Visibility: world.Visibility{
					Tier:       tier,
					KnownTo:    known,
					RumouredAs: rumour,
				},
				Source:     "human",
				Confidence: world.ConfidenceCanon,
			}
			t.Events = append(t.Events, e)
			if err := world.SaveTimeline(l, t); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s (year %d, %s)\n", e.ID, e.Year, e.Kind)
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (required).")
	cmd.Flags().IntVar(&year, "year", 0, "Year the event occurred. Defaults to Calendar.CurrentYear when omitted.")
	cmd.Flags().StringVar(&kind, "kind", "", "Event kind (war|founding|death|birth|treaty|discovery|disaster|prophecy|...). Prompted on stdin if empty.")
	cmd.Flags().StringVar(&scope, "scope", "local", "Event scope (global|regional|local|personal).")
	cmd.Flags().StringVar(&region, "region", "", "Region/place tag. Required for tier=regional events that are not common knowledge.")
	cmd.Flags().StringVar(&summary, "summary", "", "One-sentence true summary. Prompted on stdin if empty.")
	cmd.Flags().StringVar(&tier, "tier", "common", "Visibility tier: common|regional|cloistered|secret|lost.")
	cmd.Flags().StringSliceVar(&actors, "actors", nil, "Named characters / factions involved (repeatable, comma-separated).")
	cmd.Flags().StringSliceVar(&known, "known-to", nil, "For tier=cloistered: actor/faction allowlist who knows the truth.")
	cmd.Flags().StringVar(&rumour, "rumour", "", "Publicly-told distortion shown to non-knowers (regional/cloistered tiers).")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Free-form tags for grouping (repeatable).")
	cmd.Flags().StringVar(&id, "id", "", "Explicit event id. Auto-generated as evt_NNNN when omitted.")
	_ = cmd.MarkFlagRequired("slug")
	return cmd
}

// timelineReviewCommand walks proposed events one at a time and
// accepts / edits / rejects each. Designed for the
// `worldsmith timeline generate` follow-up flow where the LLM has
// just dropped 30 events into the timeline as proposed; the human
// promotes the keepers to canon.
//
// UI: per-event, prints the event + prompts `a/e/r/s/q` —
//
//	a = accept (confidence → canon)
//	e = edit (drops the event JSON into $EDITOR, accept the result)
//	r = reject (delete the event from the timeline)
//	s = skip (leave proposed, decide later)
//	q = quit
func timelineReviewCommand() *cobra.Command {
	var slug string
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Walk through proposed (LLM-generated) events one at a time; accept / edit / reject each.",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := world.Open(slug)
			if err != nil {
				return err
			}
			t, err := world.LoadTimeline(l)
			if err != nil {
				return err
			}
			rdr := bufio.NewReader(os.Stdin)
			out := cmd.OutOrStdout()
			pending := 0
			for _, e := range t.Events {
				if e.Confidence == world.ConfidenceProposed {
					pending++
				}
			}
			if pending == 0 {
				fmt.Fprintln(out, "no proposed events to review.")
				return nil
			}
			fmt.Fprintf(out, "reviewing %d proposed event(s). a=accept e=edit r=reject s=skip q=quit\n\n", pending)
			seen := 0
			accepted := 0
			rejected := 0
			skipped := 0
			out2 := make([]world.Event, 0, len(t.Events))
		eventLoop:
			for _, e := range t.Events {
				if e.Confidence != world.ConfidenceProposed {
					out2 = append(out2, e)
					continue
				}
				seen++
				fmt.Fprintf(out, "[%d/%d] %s\n", seen, pending, formatEvent(e))
				answer := strings.ToLower(strings.TrimSpace(prompt(rdr, out, "  a/e/r/s/q > ")))
				switch answer {
				case "a", "":
					e.Confidence = world.ConfidenceCanon
					out2 = append(out2, e)
					accepted++
				case "e":
					edited, err := editEventInteractive(e)
					if err != nil {
						fmt.Fprintf(out, "  edit failed: %v — keeping as proposed\n", err)
						out2 = append(out2, e)
						skipped++
						continue
					}
					edited.Confidence = world.ConfidenceCanon
					out2 = append(out2, edited)
					accepted++
				case "r":
					rejected++
					// drop — don't append
				case "s":
					out2 = append(out2, e)
					skipped++
				case "q":
					// Append the rest unchanged + break.
					out2 = append(out2, e)
					break eventLoop
				default:
					fmt.Fprintf(out, "  unknown choice %q — keeping as proposed\n", answer)
					out2 = append(out2, e)
					skipped++
				}
			}
			t.Events = out2
			if err := world.SaveTimeline(l, t); err != nil {
				return err
			}
			fmt.Fprintf(out, "\ndone. accepted=%d rejected=%d skipped=%d\n", accepted, rejected, skipped)
			return nil
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (required).")
	_ = cmd.MarkFlagRequired("slug")
	return cmd
}

// nextEventID returns the smallest unused evt_NNNN id (4 digits).
// Numbering is monotonic across the lifetime of the timeline so a
// reviewer can mentally locate "evt_0034" as roughly the 34th event
// added.
func nextEventID(events []world.Event) string {
	max := 0
	for _, e := range events {
		if !strings.HasPrefix(e.ID, "evt_") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(e.ID, "evt_"))
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return fmt.Sprintf("evt_%04d", max+1)
}

func formatEvent(e world.Event) string {
	tier := e.Visibility.Tier
	if tier == "" {
		tier = world.TierCommon
	}
	var b strings.Builder
	fmt.Fprintf(&b, "year %d  %s  tier=%s  scope=%s\n", e.Year, e.Kind, tier, e.Scope)
	fmt.Fprintf(&b, "  id:      %s\n", e.ID)
	if e.Region != "" {
		fmt.Fprintf(&b, "  region:  %s\n", e.Region)
	}
	if len(e.Actors) > 0 {
		fmt.Fprintf(&b, "  actors:  %s\n", strings.Join(e.Actors, ", "))
	}
	fmt.Fprintf(&b, "  summary: %s\n", strings.TrimSpace(e.Summary))
	if e.Visibility.RumouredAs != "" {
		fmt.Fprintf(&b, "  rumour:  %s\n", e.Visibility.RumouredAs)
	}
	if len(e.Visibility.KnownTo) > 0 {
		fmt.Fprintf(&b, "  known to: %s\n", strings.Join(e.Visibility.KnownTo, ", "))
	}
	return b.String()
}

// editEventInteractive serialises the event to JSON, pops $EDITOR
// (or $VISUAL, or `vi`) on a temp file, and re-parses the result.
// Used by `worldsmith timeline review` when the user wants to tweak
// a proposed event before promoting it to canon.
func editEventInteractive(e world.Event) (world.Event, error) {
	raw, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return e, err
	}
	tmp, err := os.CreateTemp("", "worldsmith-event-*.json")
	if err != nil {
		return e, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return e, err
	}
	if err := tmp.Close(); err != nil {
		return e, err
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// Re-use the parent stdin/stdout/stderr so the editor draws on
	// the same terminal.
	cmd := newEditorCmd(editor, tmpPath)
	if err := cmd.Run(); err != nil {
		return e, fmt.Errorf("%s exited non-zero: %w", editor, err)
	}
	updated, err := os.ReadFile(tmpPath)
	if err != nil {
		return e, err
	}
	var out world.Event
	if err := json.Unmarshal(updated, &out); err != nil {
		return e, fmt.Errorf("parse edited JSON: %w", err)
	}
	return out, nil
}

func prompt(r *bufio.Reader, w io.Writer, label string) string {
	fmt.Fprint(w, label)
	line, _ := r.ReadString('\n')
	return strings.TrimSpace(line)
}
