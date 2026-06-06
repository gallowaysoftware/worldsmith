package world

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MergeGeneratedTimeline reads the five JSON files produced by the
// timeline-gen pipeline out of runDir, merges the event lists from
// passes 2-4 with the visibility classifications from pass 5, and
// returns a slice of Events ready to be passed into
// AppendProposedEvents.
//
// The caller is responsible for actually writing them — that lets a
// CLI subcommand show the user a preview, prompt for confirmation,
// and then commit.
//
// runDir is the vamp run directory created by the pipeline; the
// pipeline writes eras.json / anchors.json / regional.json /
// personal.json / visibilities.json into it. If any of the latter
// four event files is missing, returns an error. If
// visibilities.json is missing every event simply gets a default
// `common` visibility — better than failing the whole run when fog
// was the only stage to crash.
func MergeGeneratedTimeline(runDir string) (eras []EraAnchor, events []Event, err error) {
	eras, err = readEras(filepath.Join(runDir, "eras.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("read eras: %w", err)
	}

	for _, name := range []string{"anchors.json", "regional.json", "personal.json"} {
		path := filepath.Join(runDir, name)
		batch, err := readEventBatch(path)
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", name, err)
		}
		events = append(events, batch...)
	}

	// Visibilities are best-effort: when missing, default every
	// event to common-knowledge so the human reviewer can still
	// promote them. Worse than nothing? No — a fog re-run is
	// cheap, and surfacing un-fogged events is better than
	// dropping the whole pipeline.
	visPath := filepath.Join(runDir, "visibilities.json")
	visByID, err := readVisibilities(visPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("read visibilities: %w", err)
	}
	for i := range events {
		if v, ok := visByID[events[i].ID]; ok {
			events[i].Visibility = v
		} else {
			events[i].Visibility = Visibility{Tier: TierCommon}
		}
		// Stamp source and force proposed: these are machine
		// proposals, so a model-supplied confidence:canon must not
		// bypass the `timeline review` gate.
		events[i].Source = "llm"
		events[i].Confidence = ConfidenceProposed
	}
	return eras, events, nil
}

// readEras parses eras.json into a slice of EraAnchor. The pass
// emits `{"eras":[...]}`; we unwrap the envelope.
func readEras(path string) ([]EraAnchor, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Eras []EraAnchor `json:"eras"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc.Eras, nil
}

// readEventBatch parses one of the per-pass event files. The shape
// is `{"events":[...]}`; we unwrap.
func readEventBatch(path string) ([]Event, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Events []Event `json:"events"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return doc.Events, nil
}

// readVisibilities parses visibilities.json into a lookup by event id.
func readVisibilities(path string) (map[string]Visibility, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Visibilities []struct {
			ID              string   `json:"id"`
			Tier            string   `json:"tier"`
			KnownTo         []string `json:"known_to"`
			RumouredAs      string   `json:"rumoured_as"`
			TrueFactsHidden []string `json:"true_facts_hidden"`
		} `json:"visibilities"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	out := make(map[string]Visibility, len(doc.Visibilities))
	for _, v := range doc.Visibilities {
		tier := strings.ToLower(strings.TrimSpace(v.Tier))
		switch tier {
		case TierCommon, TierRegional, TierCloistered, TierSecret, TierLost:
			// ok
		default:
			tier = TierCommon
		}
		out[v.ID] = Visibility{
			Tier:            tier,
			KnownTo:         v.KnownTo,
			RumouredAs:      v.RumouredAs,
			TrueFactsHidden: v.TrueFactsHidden,
		}
	}
	return out, nil
}
