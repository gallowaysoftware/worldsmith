package world

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// BriefFront is the optional YAML frontmatter at the top of a
// brief.md. None of the fields are required — a brief without
// frontmatter (the default) still works, it just won't carry
// timeline filtering hints.
//
// Frontmatter delimiter convention is the standard Hugo/Jekyll
// "---" before + after the YAML block, e.g.:
//
//	---
//	year_override: 412
//	pov_region: veld
//	on_stage_actors: [asha, veska]
//	---
//	The actual brief prose below the frontmatter.
//
// The body (everything after the closing `---`) is passed through to
// the writer prompt unchanged.
type BriefFront struct {
	// YearOverride, when non-zero, replaces Calendar.CurrentYear
	// for this one installment's timeline filtering. Use for
	// flashback installments set earlier than the world's "now".
	YearOverride int `yaml:"year_override,omitempty"`

	// POVRegion drives Tier=regional visibility filtering. Match
	// is case-insensitive against event.Region. Empty string
	// means "no regional gating" (every regional event passes
	// to the writer prompt as-true; not what you usually want).
	POVRegion string `yaml:"pov_region,omitempty"`

	// OnStageActors is the cast for this installment — drives
	// Tier=cloistered visibility. An event passes as-true when at
	// least one of its visibility.known_to entries is also in
	// OnStageActors; non-knowers see the rumour instead.
	OnStageActors []string `yaml:"on_stage_actors,omitempty"`
}

// ParseBrief splits brief.md into (frontmatter, body). When the
// file has no frontmatter (no leading "---"), the body is the
// entire file and BriefFront is its zero value. Errors only on a
// malformed-but-present frontmatter (trailing "---" missing, YAML
// parse failure).
func ParseBrief(path string) (BriefFront, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return BriefFront{}, "", err
	}
	front, body, err := splitFrontmatter(raw)
	if err != nil {
		return BriefFront{}, "", fmt.Errorf("%s: %w", path, err)
	}
	return front, body, nil
}

// splitFrontmatter is the pure function under ParseBrief. Split out
// for unit testing without a temp file.
func splitFrontmatter(raw []byte) (BriefFront, string, error) {
	// No frontmatter: file doesn't start with the delimiter.
	if !bytes.HasPrefix(raw, []byte("---\n")) && !bytes.HasPrefix(raw, []byte("---\r\n")) {
		return BriefFront{}, string(raw), nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var frontLines []string
	var bodyLines []string
	inFront := false
	closed := false
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			if strings.TrimSpace(line) == "---" {
				inFront = true
				continue
			}
		}
		if inFront && !closed {
			if strings.TrimSpace(line) == "---" {
				closed = true
				continue
			}
			frontLines = append(frontLines, line)
			continue
		}
		bodyLines = append(bodyLines, line)
	}
	if err := scanner.Err(); err != nil {
		return BriefFront{}, "", err
	}
	if !closed {
		return BriefFront{}, "", fmt.Errorf("frontmatter: opening --- without a closing --- before EOF")
	}
	var front BriefFront
	if err := yaml.Unmarshal([]byte(strings.Join(frontLines, "\n")), &front); err != nil {
		return BriefFront{}, "", fmt.Errorf("frontmatter YAML: %w", err)
	}
	return front, strings.Join(bodyLines, "\n"), nil
}

// FilterOptsFromBrief builds a FilterOpts for the writer prompt from
// a brief's frontmatter + the world's Calendar. Convenience so the
// caller doesn't have to remember the precedence rules:
//
//   - YearCutoff = Brief.YearOverride if set, else Calendar.CurrentYear.
//     If both are zero the cutoff is also zero, which FilterEvents
//     interprets as "no year filter" — caller may want to special-
//     case that as "everything is the future, drop everything"
//     instead, but that's a policy call.
//   - POVRegion = brief value verbatim.
//   - OnStageActors = brief value verbatim.
//   - IncludeProposed and IncludeSecret stay false (writer prompt
//     never sees secrets or unreviewed events).
func FilterOptsFromBrief(brief BriefFront, cal Calendar) FilterOpts {
	year := brief.YearOverride
	if year == 0 {
		year = cal.CurrentYear
	}
	return FilterOpts{
		YearCutoff:    year,
		POVRegion:     brief.POVRegion,
		OnStageActors: brief.OnStageActors,
	}
}

// WriteHistoricalContext computes the filtered timeline view for
// this installment and writes it as `historical_context.md` into
// the supplied run directory. Returns the absolute path of the
// written file so the caller can thread it into the pipeline as an
// input. The file always exists after this call (even when there
// are no visible events) so the writer prompt's `readFile` never
// trips a missing-file error.
func WriteHistoricalContext(runDir string, events []Event, opts FilterOpts) (string, error) {
	filtered := FilterEvents(events, opts)
	rendered := RenderForPrompt(filtered)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", err
	}
	path := runDir + string(os.PathSeparator) + "historical_context.md"
	header := "# Historical context (events visible to this installment)\n\n"
	header += fmt.Sprintf("Year cutoff: %d. Events later than this year MUST NOT be referenced — they have not happened yet from the POV character's perspective.\n\n", opts.YearCutoff)
	header += "Format per line: `year | kind | summary`. RUMOUR lines are the public distortion of events the POV character does not know the truth of — characters may believe them, the prose may state them as rumour, but treat them as suspect.\n\n"
	if err := os.WriteFile(path, []byte(header+rendered), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
