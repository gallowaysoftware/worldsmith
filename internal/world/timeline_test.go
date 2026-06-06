package world

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadTimeline_MissingFileIsEmpty(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	got, err := LoadTimeline(l)
	if err != nil {
		t.Fatalf("LoadTimeline on missing file: %v; want nil", err)
	}
	if len(got.Events) != 0 || got.Calendar.CurrentYear != 0 {
		t.Errorf("missing file should yield zero Timeline; got %#v", got)
	}
}

func TestLoadTimeline_FlatFile(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	body := Timeline{
		Calendar: Calendar{EpochLabel: "AC", CurrentYear: 412},
		Events: []Event{
			{ID: "evt_1", Year: 230, Kind: "founding", Summary: "First lighting of the salt-lamp.",
				Visibility: Visibility{Tier: TierCommon}, Confidence: ConfidenceCanon},
		},
	}
	raw, _ := json.Marshal(body)
	if err := os.WriteFile(l.TimelineFile(), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTimeline(l)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	if got.Calendar.CurrentYear != 412 || got.Calendar.EpochLabel != "AC" {
		t.Errorf("Calendar = %+v", got.Calendar)
	}
	if len(got.Events) != 1 || got.Events[0].ID != "evt_1" {
		t.Errorf("Events = %+v", got.Events)
	}
}

func TestLoadTimeline_SplitDirWins(t *testing.T) {
	// A flat timeline.json AND a timeline/ dir → split-dir wins.
	root := t.TempDir()
	l := Layout{Root: root}
	flat := Timeline{Events: []Event{{ID: "from_flat", Year: 1}}}
	flatRaw, _ := json.Marshal(flat)
	_ = os.WriteFile(l.TimelineFile(), flatRaw, 0o644)

	if err := os.Mkdir(l.TimelineDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	founding := Timeline{
		Calendar: Calendar{EpochLabel: "FE", CurrentYear: 50},
		Events:   []Event{{ID: "from_founding", Year: -10}},
	}
	conflicts := Timeline{Events: []Event{{ID: "from_conflicts", Year: 25}}}
	for name, t1 := range map[string]Timeline{
		"01_founding.json":  founding,
		"02_conflicts.json": conflicts,
	} {
		raw, _ := json.Marshal(t1)
		_ = os.WriteFile(filepath.Join(l.TimelineDir(), name), raw, 0o644)
	}

	got, err := LoadTimeline(l)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	if got.Calendar.EpochLabel != "FE" || got.Calendar.CurrentYear != 50 {
		t.Errorf("first-file-calendar-wins: got %+v", got.Calendar)
	}
	if len(got.Events) != 2 {
		t.Fatalf("expected 2 events (split-dir, flat ignored); got %d: %+v", len(got.Events), got.Events)
	}
	ids := map[string]bool{}
	for _, e := range got.Events {
		ids[e.ID] = true
	}
	if ids["from_flat"] {
		t.Errorf("flat file should be ignored when split-dir exists")
	}
	if !ids["from_founding"] || !ids["from_conflicts"] {
		t.Errorf("split-dir events missing: %v", ids)
	}
}

func TestTimelineWritable_SplitDirRejected(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	// Flat mode: writable.
	if err := TimelineWritable(l); err != nil {
		t.Errorf("flat timeline should be writable: %v", err)
	}
	// Split-dir mode: rejected, and SaveTimeline must agree (never flatten it).
	if err := os.Mkdir(l.TimelineDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := TimelineWritable(l); err == nil {
		t.Error("split-dir timeline should be reported non-writable")
	}
	if err := SaveTimeline(l, Timeline{Events: []Event{{ID: "x", Year: 1}}}); err == nil {
		t.Error("SaveTimeline should refuse to flatten a split-dir timeline")
	}
	// The split dir must be untouched (no flat timeline.json written).
	if _, err := os.Stat(l.TimelineFile()); !os.IsNotExist(err) {
		t.Errorf("SaveTimeline wrote a flat file over a split-dir layout: %v", err)
	}
}

func TestSaveTimeline_AtomicAndSorted(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	in := Timeline{
		Calendar: Calendar{CurrentYear: 100},
		Events: []Event{
			{ID: "b", Year: 50},
			{ID: "a", Year: 10},
			{ID: "c", Year: 10}, // same year as a, sort by id
		},
	}
	if err := SaveTimeline(l, in); err != nil {
		t.Fatalf("SaveTimeline: %v", err)
	}
	got, err := LoadTimeline(l)
	if err != nil {
		t.Fatalf("LoadTimeline: %v", err)
	}
	if len(got.Events) != 3 {
		t.Fatalf("event count = %d, want 3", len(got.Events))
	}
	wantOrder := []string{"a", "c", "b"}
	for i, e := range got.Events {
		if e.ID != wantOrder[i] {
			t.Errorf("position %d: id=%q, want %q", i, e.ID, wantOrder[i])
		}
	}
	// .tmp shouldn't linger.
	if _, err := os.Stat(l.TimelineFile() + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("stray .tmp file: %v", err)
	}
}

func TestAppendProposedEvents_DedupesAndStampsDefaults(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	// Existing canon event.
	if err := SaveTimeline(l, Timeline{Events: []Event{
		{ID: "evt_1", Year: 100, Confidence: ConfidenceCanon},
	}}); err != nil {
		t.Fatal(err)
	}
	added, err := AppendProposedEvents(l, []Event{
		{ID: "evt_1", Year: 100, Summary: "duplicate, should be skipped"},
		{ID: "evt_2", Year: 200, Summary: "fresh"},
		{ID: "", Year: 300, Summary: "no-id, should be skipped"},
	})
	if err != nil {
		t.Fatalf("AppendProposedEvents: %v", err)
	}
	if added != 1 {
		t.Errorf("added=%d, want 1", added)
	}
	got, _ := LoadTimeline(l)
	if len(got.Events) != 2 {
		t.Fatalf("event count = %d, want 2", len(got.Events))
	}
	var fresh *Event
	for i := range got.Events {
		if got.Events[i].ID == "evt_2" {
			fresh = &got.Events[i]
		}
	}
	if fresh == nil {
		t.Fatal("evt_2 not appended")
	}
	if fresh.Confidence != ConfidenceProposed {
		t.Errorf("Confidence = %q, want proposed", fresh.Confidence)
	}
	if fresh.Source != "llm" {
		t.Errorf("Source = %q, want llm", fresh.Source)
	}
}

func makeEvent(id string, year int, tier string, opts ...func(*Event)) Event {
	e := Event{
		ID:         id,
		Year:       year,
		Kind:       "event",
		Summary:    "summary of " + id,
		Visibility: Visibility{Tier: tier},
		Confidence: ConfidenceCanon,
	}
	for _, o := range opts {
		o(&e)
	}
	return e
}

func TestFilterEvents_YearCutoff(t *testing.T) {
	events := []Event{
		makeEvent("a", 100, TierCommon),
		makeEvent("b", 500, TierCommon),
		makeEvent("c", 1000, TierCommon),
	}
	got := FilterEvents(events, FilterOpts{YearCutoff: 500, HasCutoff: true})
	if len(got) != 2 || got[0].Event.ID != "a" || got[1].Event.ID != "b" {
		t.Errorf("year cutoff filter wrong: %+v", got)
	}

	// No cutoff requested: every year passes regardless of YearCutoff.
	got = FilterEvents(events, FilterOpts{})
	if len(got) != 3 {
		t.Errorf("HasCutoff=false should not filter by year: %+v", got)
	}

	// Epoch-zero present: events after year 0 are the future and must
	// be hidden. Previously YearCutoff==0 meant "no filter" and leaked
	// them.
	zero := []Event{
		makeEvent("now", 0, TierCommon),
		makeEvent("later", 1, TierCommon),
	}
	got = FilterEvents(zero, FilterOpts{YearCutoff: 0, HasCutoff: true})
	if len(got) != 1 || got[0].Event.ID != "now" {
		t.Errorf("epoch-zero cutoff should keep only year<=0: %+v", got)
	}
}

func TestFilterEvents_ConfidenceGate(t *testing.T) {
	events := []Event{
		makeEvent("canon", 1, TierCommon),
		{ID: "proposed", Year: 2, Kind: "event", Summary: "x", Visibility: Visibility{Tier: TierCommon}, Confidence: ConfidenceProposed},
	}
	// Writer-prompt path: proposed events are filtered out.
	got := FilterEvents(events, FilterOpts{})
	if len(got) != 1 || got[0].Event.ID != "canon" {
		t.Errorf("default filter should drop proposed events: %+v", got)
	}
	// Showrunner/debug path: proposed events surface.
	got = FilterEvents(events, FilterOpts{IncludeProposed: true})
	if len(got) != 2 {
		t.Errorf("IncludeProposed should surface both: %+v", got)
	}
}

func TestFilterEvents_VisibilityTiers(t *testing.T) {
	events := []Event{
		makeEvent("common", 1, TierCommon),
		// regional: known to anyone in "veld"
		makeEvent("veld_only", 2, TierRegional, func(e *Event) {
			e.Region = "veld"
			e.Visibility.RumouredAs = "something happened in veld"
		}),
		// regional with no rumour: non-region observers see nothing
		makeEvent("silent_regional", 3, TierRegional, func(e *Event) {
			e.Region = "marsh"
		}),
		// cloistered: only known to asha
		makeEvent("asha_secret", 4, TierCloistered, func(e *Event) {
			e.Visibility.KnownTo = []string{"asha"}
			e.Visibility.RumouredAs = "asha is hiding something"
		}),
		makeEvent("secret_event", 5, TierSecret),
		makeEvent("lost_event", 6, TierLost),
	}

	t.Run("writer prompt POV=veld with asha on stage", func(t *testing.T) {
		got := FilterEvents(events, FilterOpts{
			YearCutoff:    100,
			HasCutoff:     true,
			POVRegion:     "veld",
			OnStageActors: []string{"asha"},
		})
		// Expected:
		//  - common: pass, true
		//  - veld_only: pass, true (POV matches region)
		//  - silent_regional: drop (non-region, no rumour)
		//  - asha_secret: pass, true (asha is on stage)
		//  - secret_event: drop (no IncludeSecret)
		//  - lost_event: drop
		if len(got) != 3 {
			t.Fatalf("expected 3 events, got %d: %+v", len(got), got)
		}
		for _, fe := range got {
			if fe.RumourOnly {
				t.Errorf("expected no rumour-only entries; got %+v", fe)
			}
		}
	})

	t.Run("writer prompt POV=marsh without asha", func(t *testing.T) {
		got := FilterEvents(events, FilterOpts{YearCutoff: 100, HasCutoff: true, POVRegion: "marsh"})
		// Expected:
		//  - common: true
		//  - veld_only: rumour-only (POVRegion doesn't match, but rumour exists)
		//  - silent_regional: true (POV matches its region)
		//  - asha_secret: rumour-only (no on-stage actor; rumour exists)
		//  - secret/lost: drop
		ids := map[string]bool{}
		rumour := map[string]bool{}
		for _, fe := range got {
			ids[fe.Event.ID] = true
			if fe.RumourOnly {
				rumour[fe.Event.ID] = true
			}
		}
		if !ids["common"] || rumour["common"] {
			t.Errorf("common should pass as true; got %+v", got)
		}
		if !ids["veld_only"] || !rumour["veld_only"] {
			t.Errorf("veld_only should pass as rumour; got %+v", got)
		}
		if !ids["silent_regional"] || rumour["silent_regional"] {
			t.Errorf("silent_regional should pass as true (POV=marsh matches)")
		}
		if !ids["asha_secret"] || !rumour["asha_secret"] {
			t.Errorf("asha_secret should pass as rumour; got %+v", got)
		}
		if ids["secret_event"] || ids["lost_event"] {
			t.Errorf("secret/lost events leaked: %+v", got)
		}
	})

	t.Run("showrunner path can see secrets", func(t *testing.T) {
		got := FilterEvents(events, FilterOpts{IncludeSecret: true})
		ids := map[string]bool{}
		for _, fe := range got {
			ids[fe.Event.ID] = true
		}
		if !ids["secret_event"] {
			t.Errorf("IncludeSecret should surface secret events")
		}
		if ids["lost_event"] {
			t.Errorf("lost events should NEVER surface, even with IncludeSecret")
		}
	})
}

func TestFilterEvents_OrderingStable(t *testing.T) {
	events := []Event{
		makeEvent("z", 100, TierCommon),
		makeEvent("a", 50, TierCommon),
		makeEvent("m", 50, TierCommon),
		makeEvent("k", 100, TierCommon),
	}
	got := FilterEvents(events, FilterOpts{})
	// Year asc, then ID asc within ties: a, m, k, z
	want := []string{"a", "m", "k", "z"}
	for i, fe := range got {
		if fe.Event.ID != want[i] {
			t.Errorf("position %d: got %q, want %q", i, fe.Event.ID, want[i])
		}
	}
}

func TestRenderForPrompt(t *testing.T) {
	events := []FilteredEvent{
		{Event: makeEvent("a", 100, TierCommon, func(e *Event) {
			e.Kind = "founding"
			e.Summary = "First lighting"
		})},
		{Event: makeEvent("b", 200, TierRegional, func(e *Event) {
			e.Kind = "war"
			e.Visibility.RumouredAs = "harbour blockade"
		}), RumourOnly: true},
	}
	out := RenderForPrompt(events)
	if !strings.Contains(out, "100 | founding | First lighting") {
		t.Errorf("missing true-summary line; got: %q", out)
	}
	if !strings.Contains(out, "200 | war | RUMOUR: harbour blockade") {
		t.Errorf("missing rumour-only line; got: %q", out)
	}
}

func TestRenderForPrompt_EmptyHasPlaceholder(t *testing.T) {
	out := RenderForPrompt(nil)
	if !strings.Contains(out, "no recorded events") {
		t.Errorf("empty render should explain why; got: %q", out)
	}
}
