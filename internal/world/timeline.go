package world

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Timeline is the on-disk shape of a world's historical record. It
// composes one Calendar (the "when does this world live in") plus a
// flat Events slice. Events carry both objective facts and a
// per-event Visibility envelope that controls who-knows-what — the
// fog-of-war primitive the prose pipeline filters against per
// installment.
//
// Storage:
//   - Default: a single timeline.json at the world root.
//   - Optional split layout: timeline/<era_slug>.json files, one per
//     era. LoadTimeline concatenates all files into a single in-memory
//     value when the split-dir form is present. The user toggles by
//     moving the flat file aside and creating the dir; no schema
//     change. Worth the split once events exceed ~500 entries.
//
// JSON-only on-disk (no YAML alternate) so the generation pipeline
// can produce timeline content via vamp's `output_format: json` gate
// without a converter step.
type Timeline struct {
	Calendar Calendar `json:"calendar"`
	Events   []Event  `json:"events"`
}

// Calendar carries the per-world time scaffold: epoch label, the
// "current year" pointer the latest installment is set in, and an
// optional list of named era anchors so a long-running world can
// label its chapters of history. EraAnchors is also what the
// split-by-era storage form keys on.
type Calendar struct {
	// EpochLabel is the suffix users add to year numbers ("PE",
	// "AC", "Year of the Wolf"). Cosmetic; the engine treats
	// years as integers.
	EpochLabel string `json:"epoch_label,omitempty"`

	// CurrentYear is the "now" of the next installment. The
	// per-installment brief frontmatter may override this via
	// year_override (see FilterOpts.YearOverride).
	CurrentYear int `json:"current_year"`

	// EraAnchors is an ordered list of named eras with year
	// ranges. Optional; present for worlds that want
	// human-readable era tags on events, or for the split-by-era
	// storage form.
	EraAnchors []EraAnchor `json:"era_anchors,omitempty"`
}

// EraAnchor names a contiguous slice of history. Used both as
// metadata on events (`event.era`) and as the file-split key for the
// optional `timeline/<slug>.json` storage layout.
type EraAnchor struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Event is one historical happening. The shape is rich enough to
// drive both display (date/place/who/what) and downstream
// reasoning (cross-links + visibility), but loose enough that the
// LLM generation pipeline can populate it with `output_format: json`
// without complex schema gymnastics.
type Event struct {
	ID            string     `json:"id"`
	Year          int        `json:"year"`
	YearPrecision string     `json:"year_precision,omitempty"` // year|decade|century|legendary
	Era           string     `json:"era,omitempty"`
	Kind          string     `json:"kind"`  // war|founding|death|birth|oath|betrayal|discovery|disaster|wedding|coronation|exile|invention|prophecy|miracle|treaty|schism|other
	Scope         string     `json:"scope"` // global|regional|local|personal
	Region        string     `json:"region,omitempty"`
	Actors        []string   `json:"actors,omitempty"`
	Summary       string     `json:"summary"`
	Consequences  []string   `json:"consequences,omitempty"` // forward links to event ids
	CausedBy      []string   `json:"caused_by,omitempty"`    // backward links to event ids
	Visibility    Visibility `json:"visibility"`
	Source        string     `json:"source"`     // human|llm
	Confidence    string     `json:"confidence"` // canon|proposed
	Tags          []string   `json:"tags,omitempty"`
}

// Visibility is the fog-of-war envelope on every event. Drives
// whether the event gets surfaced to the writer prompt as fact, as
// rumour, or not at all — see FilterEvents.
type Visibility struct {
	// Tier classifies the event's reach. Allowed values
	// (least-secret → most-secret):
	//   common      → everyone in-world knows it. Always surfaces.
	//   regional    → known within Region; non-Region installments
	//                 see RumouredAs if non-empty, else nothing.
	//   cloistered  → known only to actors in KnownTo. Non-knowers
	//                 see RumouredAs if non-empty.
	//   secret      → never surfaced to the writer prompt. Optionally
	//                 surfaces to the showrunner / brief layer as a
	//                 dramatic-reveal hint.
	//   lost        → not surfaced anywhere; survives only via direct
	//                 brief authoring (the human knows; the LLM does
	//                 not).
	Tier string `json:"tier"`

	// KnownTo is the actor / faction allowlist for cloistered tier
	// events. Ignored for common/regional (the latter uses Region)
	// and for secret/lost (no one in-prose knows).
	KnownTo []string `json:"known_to,omitempty"`

	// RumouredAs is the publicly-told distortion of the event,
	// shown to non-knowers in prompts as "RUMOUR: ..." instead of
	// the true summary. Empty string means "no rumour exists" and
	// non-knowers see nothing about the event.
	RumouredAs string `json:"rumoured_as,omitempty"`

	// TrueFactsHidden enumerates what RumouredAs leaves out.
	// Informational; not surfaced to the writer prompt. Useful for
	// human review and for showrunner-level reveal hints.
	TrueFactsHidden []string `json:"true_facts_hidden,omitempty"`
}

// Valid tier values.
const (
	TierCommon     = "common"
	TierRegional   = "regional"
	TierCloistered = "cloistered"
	TierSecret     = "secret"
	TierLost       = "lost"
)

const (
	ConfidenceCanon    = "canon"
	ConfidenceProposed = "proposed"
)

// TimelineFile returns the canonical flat path. Even when the split-dir
// layout is in use, this is the path `worldsmith timeline add` writes
// to when the directory form is absent.
func (l Layout) TimelineFile() string { return filepath.Join(l.Root, "timeline.json") }

// TimelineDir returns the optional split-storage directory. Present →
// LoadTimeline concatenates `timeline/*.json` instead of reading the
// flat file. Absent → flat-file path wins.
func (l Layout) TimelineDir() string { return filepath.Join(l.Root, "timeline") }

// LoadTimeline reads the world's timeline from disk. Resolution order:
//
//  1. If the split-dir `timeline/` exists, concatenate every
//     `*.json` file inside it (sorted lexically by filename, so
//     numeric-prefixed era files like `01_founding.json` come first).
//     The first file's Calendar wins; subsequent files' calendar
//     blocks are ignored — only events are merged.
//  2. Else if the flat `timeline.json` exists, parse it.
//  3. Else return an empty Timeline + nil error so a fresh world
//     without a timeline doesn't break downstream consumers.
//
// Loading is forgiving by design: unknown JSON keys are silently
// dropped (encoding/json default), an unreadable file in the
// split-dir is wrapped with its path so the user knows which file
// broke parse.
func LoadTimeline(l Layout) (Timeline, error) {
	if info, err := os.Stat(l.TimelineDir()); err == nil && info.IsDir() {
		return loadTimelineSplit(l)
	}
	raw, err := os.ReadFile(l.TimelineFile())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Timeline{}, nil
		}
		return Timeline{}, err
	}
	var t Timeline
	if err := json.Unmarshal(raw, &t); err != nil {
		return Timeline{}, fmt.Errorf("parse %s: %w", l.TimelineFile(), err)
	}
	return t, nil
}

func loadTimelineSplit(l Layout) (Timeline, error) {
	entries, err := os.ReadDir(l.TimelineDir())
	if err != nil {
		return Timeline{}, err
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	var merged Timeline
	calendarSet := false
	for _, name := range files {
		path := filepath.Join(l.TimelineDir(), name)
		raw, err := os.ReadFile(path)
		if err != nil {
			return Timeline{}, fmt.Errorf("read %s: %w", path, err)
		}
		var part Timeline
		if err := json.Unmarshal(raw, &part); err != nil {
			return Timeline{}, fmt.Errorf("parse %s: %w", path, err)
		}
		// First file wins the calendar so subsequent era files
		// don't have to repeat it; treat their calendar blocks
		// as ignorable.
		if !calendarSet && (part.Calendar.CurrentYear != 0 || part.Calendar.EpochLabel != "" || len(part.Calendar.EraAnchors) > 0) {
			merged.Calendar = part.Calendar
			calendarSet = true
		}
		merged.Events = append(merged.Events, part.Events...)
	}
	return merged, nil
}

// SaveTimeline writes the in-memory Timeline back to disk. Only the
// flat-file form is written — if you maintain a split-dir layout by
// hand, use SaveEvents to append proposed events to a specific file
// of your choice, or merge + re-split manually.
//
// Guard: when the split-dir `timeline/` exists, LoadTimeline reads from
// it and ignores the flat file, so a flat-file write would be silently
// lost on the next load. SaveTimeline refuses that case with an explicit
// error rather than dropping the mutation.
//
// Atomic via tmp-then-rename so a crash during write doesn't leave a
// half-written timeline.json.
func SaveTimeline(l Layout, t Timeline) error {
	if info, err := os.Stat(l.TimelineDir()); err == nil && info.IsDir() {
		return fmt.Errorf("timeline %q is in split-dir mode; SaveTimeline only writes the flat file — merge timeline/ to a flat timeline.json first", l.TimelineDir())
	}
	if err := os.MkdirAll(l.Root, 0o755); err != nil {
		return err
	}
	// Stable event ordering: sort by year, then by id, before write.
	// Keeps diffs reviewable across re-saves.
	sort.SliceStable(t.Events, func(i, j int) bool {
		if t.Events[i].Year != t.Events[j].Year {
			return t.Events[i].Year < t.Events[j].Year
		}
		return t.Events[i].ID < t.Events[j].ID
	})
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	final := l.TimelineFile()
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// AppendProposedEvents merges newly-LLM-generated events into the
// timeline with Confidence=proposed and Source=llm. Existing events
// with matching IDs are left alone; the human gates promotion to
// canon through the `worldsmith timeline review` subcommand. Returns
// the count of events added (some may have been deduped by ID).
//
// Used by the timeline-generation pipeline's final write step.
func AppendProposedEvents(l Layout, fresh []Event) (added int, err error) {
	t, err := LoadTimeline(l)
	if err != nil {
		return 0, err
	}
	existing := make(map[string]bool, len(t.Events))
	for _, e := range t.Events {
		existing[e.ID] = true
	}
	for _, e := range fresh {
		if e.ID == "" || existing[e.ID] {
			continue
		}
		if e.Confidence == "" {
			e.Confidence = ConfidenceProposed
		}
		if e.Source == "" {
			e.Source = "llm"
		}
		t.Events = append(t.Events, e)
		existing[e.ID] = true
		added++
	}
	return added, SaveTimeline(l, t)
}

// FilterOpts narrows a Timeline's Events to the subset visible to a
// given perspective at a given moment in narrative time. The
// installment-prep code populates it from brief frontmatter +
// characters-on-stage + region tag; the pipeline then injects the
// filtered slice into the writer prompt.
type FilterOpts struct {
	// YearCutoff is the inclusive upper bound on event.Year — only
	// events whose year is <= this value are returned (and only when
	// HasCutoff is set). Set to the brief's year_override if present,
	// else to Calendar.CurrentYear.
	YearCutoff int

	// HasCutoff distinguishes "no year filter" from "cut off at year
	// zero". An epoch-zero calendar (CurrentYear == 0) is a legitimate
	// narrative present whose future events must still be hidden, so we
	// can't overload YearCutoff==0 to mean "no filter".
	HasCutoff bool

	// POVRegion is the region the installment is set in. Used to
	// gate Tier=regional events; events from other regions surface
	// only via RumouredAs (or not at all if no rumour exists).
	// Empty string means "any region passes" (no filter).
	POVRegion string

	// OnStageActors is the union of named characters + factions
	// the installment foregrounds. Drives Tier=cloistered visibility:
	// an event passes if any of its KnownTo entries is also in
	// OnStageActors. Empty slice means "no one is on-stage"
	// (cloistered events fall to RumouredAs path).
	OnStageActors []string

	// IncludeProposed lets the showrunner / debugging paths see
	// proposed events too. Writer prompts always set this false —
	// only canon-promoted events become narrative truth.
	IncludeProposed bool

	// IncludeSecret lets the showrunner / brief planning paths see
	// secret events as candidate dramatic reveals. Writer prompts
	// always set this false (the writer never sees secrets, even
	// when the POV character would).
	IncludeSecret bool
}

// FilteredEvent is the result of running an event through
// FilterEvents — either the true summary (when the POV can see it) or
// the rumoured distortion. Callers can render the two cases
// differently in the prompt ("Year 412: ..." vs "Year 412 (RUMOUR):
// ...").
type FilteredEvent struct {
	Event Event
	// RumourOnly is true when the event was surfaced via its
	// RumouredAs text rather than its true Summary (the POV knows
	// the rumour, not the fact). The prompt rendering for these
	// events should use Event.Visibility.RumouredAs in place of
	// Event.Summary.
	RumourOnly bool
}

// FilterEvents applies confidence + year + visibility filtering and
// returns the events in chronological order. Visibility rules:
//
//   - Tier=common      → always passes with true summary.
//   - Tier=regional    → true summary when opts.POVRegion matches
//     event.Region; otherwise rumour-only path
//     (RumouredAs surfaced if present, else
//     dropped).
//   - Tier=cloistered  → true summary when any of event.Visibility.KnownTo
//     is in opts.OnStageActors; otherwise rumour-only
//     path. Empty OnStageActors falls to rumour-only.
//   - Tier=secret      → dropped unless opts.IncludeSecret (showrunner
//     layer only), in which case true summary.
//   - Tier=lost        → always dropped.
//
// An unrecognised tier is surfaced as a plain common-equivalent event
// with no annotation (safer than dropping). Events with empty
// visibility tier are likewise treated as common.
//
// Output ordering: by Year ascending, then by ID for ties (stable
// against re-saves).
func FilterEvents(events []Event, opts FilterOpts) []FilteredEvent {
	onStage := make(map[string]bool, len(opts.OnStageActors))
	for _, a := range opts.OnStageActors {
		onStage[a] = true
	}
	var out []FilteredEvent
	for _, e := range events {
		// Year cutoff: skip future events.
		if opts.HasCutoff && e.Year > opts.YearCutoff {
			continue
		}
		// Confidence gate.
		switch e.Confidence {
		case ConfidenceCanon, "": // empty defaults to canon for hand-authored files
			// passes
		case ConfidenceProposed:
			if !opts.IncludeProposed {
				continue
			}
		default:
			// Unknown confidence value — drop to be safe.
			continue
		}
		tier := e.Visibility.Tier
		if tier == "" {
			tier = TierCommon
		}
		switch tier {
		case TierLost:
			continue
		case TierSecret:
			if !opts.IncludeSecret {
				continue
			}
			out = append(out, FilteredEvent{Event: e})
		case TierCommon:
			out = append(out, FilteredEvent{Event: e})
		case TierRegional:
			if opts.POVRegion == "" || strings.EqualFold(opts.POVRegion, e.Region) {
				out = append(out, FilteredEvent{Event: e})
			} else if e.Visibility.RumouredAs != "" {
				out = append(out, FilteredEvent{Event: e, RumourOnly: true})
			}
		case TierCloistered:
			anyKnown := false
			for _, k := range e.Visibility.KnownTo {
				if onStage[k] {
					anyKnown = true
					break
				}
			}
			if anyKnown {
				out = append(out, FilteredEvent{Event: e})
			} else if e.Visibility.RumouredAs != "" {
				out = append(out, FilteredEvent{Event: e, RumourOnly: true})
			}
		default:
			// Unknown tier: surface conservatively as common-equivalent.
			out = append(out, FilteredEvent{Event: e})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Event.Year != out[j].Event.Year {
			return out[i].Event.Year < out[j].Event.Year
		}
		return out[i].Event.ID < out[j].Event.ID
	})
	return out
}

// RenderForPrompt formats a filtered event list into the
// "Historical context" block that ships into the writer prompt. One
// line per event:
//
//	<year> | <kind> | <summary>
//	<year> | <kind> | RUMOUR: <rumoured_as>
//
// Caller is responsible for the section header and the framing
// sentence about what the POV character is allowed to reference.
func RenderForPrompt(events []FilteredEvent) string {
	if len(events) == 0 {
		return "(no recorded events the POV character would know.)"
	}
	var b strings.Builder
	for _, fe := range events {
		summary := fe.Event.Summary
		if fe.RumourOnly {
			summary = "RUMOUR: " + fe.Event.Visibility.RumouredAs
		}
		kind := fe.Event.Kind
		if kind == "" {
			kind = "event"
		}
		fmt.Fprintf(&b, "%d | %s | %s\n", fe.Event.Year, kind, strings.TrimSpace(summary))
	}
	return b.String()
}
