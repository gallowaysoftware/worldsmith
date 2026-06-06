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

	// Reveals lists the sealed notebook material this installment is
	// LICENSED to bring onto the page — dossier slugs and/or short
	// phrases naming the specific secret to surface. Everything else in
	// the notebook stays sealed (subtext only). Items a dossier marks
	// NEVER / honour-by-absence are never licensable, even if listed
	// here. Drives the writer's reveal permission and the fog-check's
	// allow-list. Empty (the default) = reveal nothing sealed.
	Reveals []string `yaml:"reveals,omitempty"`

	// TargetWords, when non-zero, sets this installment's target prose
	// length (drives the outline's per-scene budgets). Empty = the
	// pipeline default (~10,000). The series flow sets it per book so a
	// chapter lands at ~5.5k; story/novel leave it 0 (default length).
	TargetWords int `yaml:"target_words,omitempty"`
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
//     The cutoff always applies (HasCutoff is always true): the brief
//     defines a narrative present, so events after it are the future
//     and must be hidden — including the legitimate epoch-zero case
//     where both YearOverride and CurrentYear are 0.
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
		HasCutoff:     true,
		POVRegion:     brief.POVRegion,
		OnStageActors: brief.OnStageActors,
	}
}

// BriefTitle reads the brief's body and returns its descriptive
// title — the first H1 heading, with any leading installment-number
// prefix stripped. Designed for filename construction in the
// `--publish-to` flow: a brief whose H1 is `# 001 — The First Hour`
// returns `The First Hour`, and the caller wraps that as
// `001 - The First Hour.m4b`. A brief without an H1 (or whose H1 is
// only the installment number) returns the empty string; callers
// should fall back to a numeric default.
//
// The strip-prefix logic handles three common patterns: `001 — Foo`,
// `001 - Foo`, `001. Foo` (em-dash, hyphen, period separators). A
// title that doesn't lead with the installment number is returned
// unchanged.
//
// Returns the empty string on read error or no H1 found; the empty
// string is the "fall back to default" signal, not an error.
func BriefTitle(path string) string {
	_, body, err := ParseBrief(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "# ") {
			continue
		}
		title := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		title = stripInstallmentPrefix(title)
		return title
	}
	return ""
}

// stripInstallmentPrefix removes a leading "NNN — " / "NNN - " /
// "NNN. " pattern from a brief title. Hand-written authors often
// include the installment number in the H1 ("# 001 — The First
// Hour"); the filename already carries `%03d -` so the title
// component shouldn't duplicate it.
func stripInstallmentPrefix(title string) string {
	// Walk past any leading digits.
	i := 0
	for i < len(title) && title[i] >= '0' && title[i] <= '9' {
		i++
	}
	if i == 0 {
		return title
	}
	rest := strings.TrimLeft(title[i:], " ")
	// Accept em-dash, hyphen, or period as the separator.
	for _, sep := range []string{"— ", "– ", "- ", ". "} {
		if strings.HasPrefix(rest, sep) {
			return strings.TrimSpace(rest[len(sep):])
		}
	}
	return title
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

// WriteLicensedReveals renders this installment's licensed-reveal allow-list into
// runDir/licensed_reveals.md and returns the path. The writer reads it to know which
// sealed notebook material it MAY surface this installment; the fog-check reads it as
// the allow-list of permitted reveals. The file always exists after this call (a
// "none" sentinel when the brief licenses nothing) so the prompts' readFile never
// trips a missing-file error.
func WriteLicensedReveals(runDir string, reveals []string) (string, error) {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# Licensed reveals for this installment\n\n")
	if len(reveals) == 0 {
		b.WriteString("None. Reveal nothing sealed this installment — every notebook secret stays subtext only.\n")
	} else {
		b.WriteString("The brief LICENSES the following sealed material to be shown on the page in THIS installment. ")
		b.WriteString("Everything else in the notebook stays sealed (subtext only). ")
		b.WriteString("Items a dossier marks NEVER / honour-by-absence are never licensed, even if listed below.\n\n")
		for _, r := range reveals {
			r = strings.TrimSpace(r)
			if r == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}
	path := runDir + string(os.PathSeparator) + "licensed_reveals.md"
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
