package world

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/gallowaysoftware/vibe/contentkit"
)

// stripCodeFence removes a leading ```/```json line and a trailing ``` fence that a
// model sometimes wraps JSON in despite OutputFormatJSON. Returns the inner bytes
// (trimmed). No-op when there's no fence.
func stripCodeFence(t []byte) []byte {
	if !bytes.HasPrefix(t, []byte("```")) {
		return t
	}
	if i := bytes.IndexByte(t, '\n'); i >= 0 {
		t = t[i+1:]
	}
	t = bytes.TrimSpace(t)
	t = bytes.TrimSuffix(t, []byte("```"))
	return bytes.TrimSpace(t)
}

// countFindingsJSON parses a JSON check report ({"findings":[{"severity":…}]}) into
// per-severity counts (upper-cased). ok=false when the bytes aren't a JSON object, so
// the caller falls back to the legacy markdown Verdict-line regex for old reports.
func countFindingsJSON(raw []byte) (map[string]int, bool) {
	t := stripCodeFence(bytes.TrimSpace(raw))
	if len(t) == 0 || t[0] != '{' {
		return nil, false
	}
	var rep struct {
		Findings []struct {
			Severity string `json:"severity"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(t, &rep); err != nil {
		return nil, false
	}
	counts := map[string]int{}
	for _, f := range rep.Findings {
		counts[strings.ToUpper(strings.TrimSpace(f.Severity))]++
	}
	return counts, true
}

// This file turns worldsmith's existing per-installment signals (the deterministic
// prose metrics + the continuity report) into contentkit.ScoreResults aggregated
// into a Scorecard, written to scorecard.json beside the m4b. The point is to make
// quality a tracked number across installments — measurement, not vibes.

// ProseScoreResult converts deterministic prose metrics into a 0-100 quality
// score with located violations. The weighting is deliberately simple and
// conservative; it's a trend signal, not a gate.
func ProseScoreResult(m ProseMetrics) contentkit.ScoreResult {
	score := 100.0
	metrics := map[string]float64{
		"words":             float64(m.Words),
		"slop_per_1000":     m.SlopPer1000,
		"not_x_but_y":       float64(m.NotXButY),
		"repeated_trigrams": float64(len(m.RepeatedTrigrams)),
		"repeated_openers":  float64(len(m.RepeatedOpeners)),
	}
	var v []contentkit.Violation

	// Slop density: ~0/1k ideal; penalise up to -40.
	if p := m.SlopPer1000 * 8; p > 0 {
		if p > 40 {
			p = 40
		}
		score -= p
		if m.SlopPer1000 >= 2 {
			v = append(v, contentkit.Violation{Severity: "minor",
				Message: fmt.Sprintf("slop density %.1f/1k (%d hits)", m.SlopPer1000, m.SlopTotal),
				Excerpt: topSlop(m.SlopHits)})
		}
	}
	// "not X but Y" reflex, rate-normalised.
	if m.Words > 0 {
		nxby := float64(m.NotXButY) * 1000.0 / float64(m.Words)
		if p := nxby * 10; p > 0 {
			if p > 20 {
				p = 20
			}
			score -= p
		}
		if m.NotXButY >= 6 {
			v = append(v, contentkit.Violation{Severity: "minor",
				Message: fmt.Sprintf("%d 'not X but Y' constructions", m.NotXButY)})
		}
	}
	// Looping phrases / anaphora. Repeated trigrams include function-word noise
	// ("out of the"), so they carry low weight; repeated sentence openers are
	// anaphora collapse ("He thought of X. He thought of Y."), a sharper tell.
	score -= clampf(float64(len(m.RepeatedTrigrams)), 0, 8)
	score -= clampf(float64(len(m.RepeatedOpeners))*2, 0, 12)
	for _, o := range m.RepeatedOpeners {
		if o.Count >= 6 {
			v = append(v, contentkit.Violation{Severity: "minor",
				Message: fmt.Sprintf("sentence opener repeated x%d (anaphora)", o.Count), Excerpt: o.Phrase})
		}
	}

	return contentkit.ScoreResult{
		Score:      int(clampf(score, 0, 100)),
		Metrics:    metrics,
		Violations: v,
		Summary: fmt.Sprintf("%d words, slop %.1f/1k, not-x-but-y %d, repeats %d/%d",
			m.Words, m.SlopPer1000, m.NotXButY, len(m.RepeatedTrigrams), len(m.RepeatedOpeners)),
	}
}

var continuityVerdictRe = regexp.MustCompile(`(?i)(\d+)\s+breaking,\s*(\d+)\s+minor(?:,\s*(\d+)\s+watch)?`)

// ContinuityScoreResult parses a continuity_report.md verdict line into a score.
// CLEAN = 100; each breaking -30, minor -8, watch -2.
func ContinuityScoreResult(reportPath string) contentkit.ScoreResult {
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		return contentkit.ScoreResult{Score: 0, Summary: "no continuity report"}
	}
	if counts, ok := countFindingsJSON(raw); ok {
		breaking, minor, watch := counts["BREAKING"], counts["MINOR"], counts["WATCH"]
		score := 100 - 30*breaking - 8*minor - 2*watch
		v := []contentkit.Violation{}
		if breaking > 0 {
			v = append(v, contentkit.Violation{Severity: "breaking", Message: fmt.Sprintf("%d breaking continuity issue(s)", breaking)})
		}
		if minor > 0 {
			v = append(v, contentkit.Violation{Severity: "minor", Message: fmt.Sprintf("%d minor continuity issue(s)", minor)})
		}
		return contentkit.ScoreResult{
			Score:      clampi(score, 0, 100),
			Metrics:    map[string]float64{"breaking": float64(breaking), "minor": float64(minor), "watch": float64(watch)},
			Violations: v,
			Summary:    fmt.Sprintf("%d breaking, %d minor, %d watch", breaking, minor, watch),
		}
	}
	text := string(raw)
	if strings.Contains(strings.ToUpper(text), "VERDICT:") && strings.Contains(strings.ToUpper(text), "CLEAN") &&
		!continuityVerdictRe.MatchString(text) {
		return contentkit.ScoreResult{Score: 100, Summary: "clean"}
	}
	mm := continuityVerdictRe.FindStringSubmatch(text)
	if mm == nil {
		return contentkit.ScoreResult{Score: 100, Summary: "clean"}
	}
	breaking, minor, watch := atoi(mm[1]), atoi(mm[2]), atoi(mm[3])
	score := 100 - 30*breaking - 8*minor - 2*watch
	v := []contentkit.Violation{}
	if breaking > 0 {
		v = append(v, contentkit.Violation{Severity: "breaking", Message: fmt.Sprintf("%d breaking continuity issue(s)", breaking)})
	}
	if minor > 0 {
		v = append(v, contentkit.Violation{Severity: "minor", Message: fmt.Sprintf("%d minor continuity issue(s)", minor)})
	}
	return contentkit.ScoreResult{
		Score:      clampi(score, 0, 100),
		Metrics:    map[string]float64{"breaking": float64(breaking), "minor": float64(minor), "watch": float64(watch)},
		Violations: v,
		Summary:    fmt.Sprintf("%d breaking, %d minor, %d watch", breaking, minor, watch),
	}
}

var fogVerdictRe = regexp.MustCompile(`(?i)(\d+)\s+leak(?:s)?,\s*(\d+)\s+watch`)

// FogScoreResult parses a fog_report.md verdict into a score. A stated LEAK is a
// shipped spoiler — sealed material put on the page — so it weighs heavily (-30
// each, like a breaking continuity error); each WATCH -5. CLEAN = 100. A missing
// report yields a clean 100 (the installment predates fog-checking); BuildScorecard
// only includes this axis when the report exists, so it never drags an old card.
func FogScoreResult(reportPath string) contentkit.ScoreResult {
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		return contentkit.ScoreResult{Score: 100, Summary: "no fog report"}
	}
	if counts, ok := countFindingsJSON(raw); ok {
		leaks, watch := counts["LEAK"], counts["WATCH"]
		score := 100 - 30*leaks - 5*watch
		v := []contentkit.Violation{}
		if leaks > 0 {
			v = append(v, contentkit.Violation{Severity: "breaking",
				Message: fmt.Sprintf("%d fog-of-war leak(s) — sealed material stated on the page", leaks)})
		}
		if watch > 0 {
			v = append(v, contentkit.Violation{Severity: "minor", Message: fmt.Sprintf("%d fog watch(es)", watch)})
		}
		return contentkit.ScoreResult{
			Score:      clampi(score, 0, 100),
			Metrics:    map[string]float64{"leaks": float64(leaks), "watch": float64(watch)},
			Violations: v,
			Summary:    fmt.Sprintf("%d leak, %d watch", leaks, watch),
		}
	}
	mm := fogVerdictRe.FindStringSubmatch(string(raw))
	if mm == nil {
		// CLEAN verdict (no parseable leak/watch counts) → nothing leaked.
		return contentkit.ScoreResult{Score: 100, Summary: "clean"}
	}
	leaks, watch := atoi(mm[1]), atoi(mm[2])
	score := 100 - 30*leaks - 5*watch
	v := []contentkit.Violation{}
	if leaks > 0 {
		v = append(v, contentkit.Violation{Severity: "breaking",
			Message: fmt.Sprintf("%d fog-of-war leak(s) — sealed material stated on the page", leaks)})
	}
	if watch > 0 {
		v = append(v, contentkit.Violation{Severity: "minor", Message: fmt.Sprintf("%d fog watch(es)", watch)})
	}
	return contentkit.ScoreResult{
		Score:      clampi(score, 0, 100),
		Metrics:    map[string]float64{"leaks": float64(leaks), "watch": float64(watch)},
		Violations: v,
		Summary:    fmt.Sprintf("%d leak, %d watch", leaks, watch),
	}
}

// BuildScorecard assembles the scorecard for installment n: prose (deterministic)
// + continuity + fog-of-war (each parsed from its report). Recomputes prose from
// story.md so it works even on installments generated before scorecards existed.
func BuildScorecard(l Layout, n int) contentkit.Scorecard {
	results := make([]contentkit.ScoreResult, 0, 3)
	if raw, err := os.ReadFile(l.InstallmentFile(n, "story.md")); err == nil {
		results = append(results, named("prose", ProseScoreResult(AnalyzeProse(string(raw)))))
	}
	results = append(results, named("continuity", ContinuityScoreResult(l.InstallmentFile(n, "continuity_report.md"))))
	// Fog-of-war is an axis only when the installment was fog-checked (the report
	// exists). Installments generated before fog-checking simply lack the dimension.
	if fogPath := l.InstallmentFile(n, "fog_report.md"); fileExistsWS(fogPath) {
		results = append(results, named("fog", FogScoreResult(fogPath)))
	}
	return contentkit.Scorecard{Item: fmt.Sprintf("%03d", n), Results: results}
}

func fileExistsWS(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// WriteScorecard builds and writes scorecard.json for installment n. Non-fatal:
// returns the card + any write error.
func WriteScorecard(l Layout, n int) (contentkit.Scorecard, error) {
	card := BuildScorecard(l, n)
	out, err := json.MarshalIndent(card, "", "  ")
	if err != nil {
		return card, err
	}
	return card, os.WriteFile(l.InstallmentFile(n, "scorecard.json"), out, 0o644)
}

// Overall is the composite score for a card: the minimum across its results
// (one bad axis shouldn't be hidden by a good average).
func Overall(card contentkit.Scorecard) int {
	if len(card.Results) == 0 {
		return 0
	}
	min := 100
	for _, r := range card.Results {
		if r.Score < min {
			min = r.Score
		}
	}
	return min
}

// --- helpers ---

func named(name string, r contentkit.ScoreResult) contentkit.ScoreResult {
	if r.Summary != "" {
		r.Summary = name + ": " + r.Summary
	}
	return r
}

func topSlop(hits map[string]int) string {
	var parts []string
	for k, c := range hits {
		parts = append(parts, fmt.Sprintf("%s×%d", k, c))
	}
	return strings.Join(parts, ", ")
}

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}
