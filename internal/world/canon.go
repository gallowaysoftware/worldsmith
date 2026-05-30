package world

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Canon retrieval.
//
// canon.md grows monotonically — every installment appends its
// canon_delta. By installment 20 that's a wall of facts, most of them
// irrelevant to the brief in hand, all of it dumped into every prose
// prompt: wasted context, diluted attention, repeated token cost.
//
// SelectRelevantCanon trims that wall down to the facts a given
// installment actually needs, the same way the timeline's fog-of-war
// filter trims history. It is deliberately conservative: for a small
// canon it changes nothing (returns the document verbatim), and even
// when it filters it always keeps world rules and any fact naming an
// on-stage actor — the entries whose omission would actually break
// continuity. The full canon.md is untouched on disk and still feeds
// the canon_delta + continuity stages, which need the whole picture.

// canonEntry is one bullet fact plus the headers it sits under.
type canonEntry struct {
	installment string // "## From installment 3" line, verbatim (may be empty)
	category    string // "## People" line, verbatim (may be empty)
	lines       []string
	score       int
	keep        bool // forced-keep (rule or names an actor)
}

func (e canonEntry) text() string { return strings.ToLower(strings.Join(e.lines, " ")) }

var (
	canonHeaderRe      = regexp.MustCompile(`^#{1,6}\s+`)
	canonInstallmentRe = regexp.MustCompile(`(?i)^#{1,6}\s+from installment\b`)
	canonRuleRe        = regexp.MustCompile(`(?i)\brule`)
	canonWordRe        = regexp.MustCompile(`[\p{L}']+`)
)

// SelectRelevantCanon returns a view of canon scoped to a brief. If
// canon is at or under maxChars it is returned unchanged. Otherwise it
// is parsed into entries, scored against the brief body + on-stage
// actor names, and re-emitted keeping (a) every rule, (b) every entry
// naming an actor, and (c) the highest-scoring remainder until the
// output approaches maxChars. Header structure is preserved for the
// kept entries.
func SelectRelevantCanon(canon, briefBody string, actors []string, maxChars int) string {
	if len(canon) <= maxChars || strings.TrimSpace(canon) == "" {
		return canon
	}

	entries, preamble := parseCanon(canon)
	if len(entries) == 0 {
		return canon
	}

	query := queryTokens(briefBody)
	lowerActors := make([]string, 0, len(actors))
	for _, a := range actors {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			lowerActors = append(lowerActors, a)
		}
	}

	for i := range entries {
		e := &entries[i]
		body := e.text()
		// Forced keep: world rules, and anything naming an on-stage actor.
		if canonRuleRe.MatchString(e.category) {
			e.keep = true
		}
		for _, a := range lowerActors {
			if strings.Contains(body, a) {
				e.keep = true
				e.score += 10
			}
		}
		toks := canonWordRe.FindAllString(body, -1)
		for _, t := range toks {
			if query[t] {
				e.score++
			}
		}
	}

	// Order the non-forced entries by score so we fill the budget with
	// the most relevant first. Forced-keep entries are always in.
	idx := make([]int, len(entries))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ea, eb := entries[idx[a]], entries[idx[b]]
		if ea.keep != eb.keep {
			return ea.keep // forced-keep first
		}
		return ea.score > eb.score
	})

	chosen := map[int]bool{}
	budget := maxChars
	for _, i := range idx {
		e := entries[i]
		sz := len(strings.Join(e.lines, "\n")) + 1
		if e.keep || e.score > 0 {
			if !e.keep && sz > budget {
				continue
			}
			chosen[i] = true
			budget -= sz
		}
		if budget <= 0 {
			break
		}
	}

	return renderCanon(entries, chosen, preamble, len(query) > 0)
}

// parseCanon walks canon.md into entries. Any leading content before
// the first header is returned as preamble. Bullets ("- ...") start a
// new entry; indented continuation lines attach to the current entry.
func parseCanon(canon string) (entries []canonEntry, preamble string) {
	lines := strings.Split(canon, "\n")
	var inst, cat string
	var cur *canonEntry
	var pre []string
	seenHeader := false

	flush := func() {
		if cur != nil {
			entries = append(entries, *cur)
			cur = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case canonInstallmentRe.MatchString(trimmed):
			flush()
			inst = trimmed
			cat = ""
			seenHeader = true
		case canonHeaderRe.MatchString(trimmed):
			flush()
			cat = trimmed
			seenHeader = true
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			flush()
			cur = &canonEntry{installment: inst, category: cat, lines: []string{line}}
		case cur != nil && trimmed != "":
			cur.lines = append(cur.lines, line)
		case !seenHeader && trimmed != "":
			pre = append(pre, line)
		default:
			// blank line ends a multi-line entry's continuation
			flush()
		}
	}
	flush()
	return entries, strings.TrimSpace(strings.Join(pre, "\n"))
}

// renderCanon re-emits the chosen entries with their headers,
// preserving order and only printing a header when it changes.
func renderCanon(entries []canonEntry, chosen map[int]bool, preamble string, filtered bool) string {
	var b strings.Builder
	if filtered {
		b.WriteString("<!-- Relevance-filtered view of canon for this installment. ")
		b.WriteString("World rules and facts about on-stage characters are always included; ")
		b.WriteString("other facts are the ones most relevant to this brief. -->\n\n")
	}
	if preamble != "" {
		b.WriteString(preamble)
		b.WriteString("\n\n")
	}
	var lastInst, lastCat string
	for i, e := range entries {
		if !chosen[i] {
			continue
		}
		if e.installment != "" && e.installment != lastInst {
			fmt.Fprintf(&b, "%s\n\n", e.installment)
			lastInst = e.installment
			lastCat = "" // category repeats under a new installment
		}
		if e.category != "" && e.category != lastCat {
			fmt.Fprintf(&b, "%s\n", e.category)
			lastCat = e.category
		}
		b.WriteString(strings.Join(e.lines, "\n"))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// queryTokens builds the set of meaningful tokens (length >= 4,
// lowercased) from a brief body to score canon entries against.
func queryTokens(briefBody string) map[string]bool {
	q := map[string]bool{}
	for _, t := range canonWordRe.FindAllString(strings.ToLower(briefBody), -1) {
		if len(t) >= 4 {
			q[t] = true
		}
	}
	return q
}

// WriteRelevantCanon computes the relevance-filtered canon for an
// installment and writes it as canon_relevant.md into runDir, returning
// the path. The full canon is read from canonPath. maxChars gates
// filtering: at or under it, the file is a verbatim copy (so prompts
// can read a single canon_relevant_file input in every case).
func WriteRelevantCanon(runDir, canonPath, briefBody string, actors []string, maxChars int) (string, error) {
	raw, err := os.ReadFile(canonPath)
	if err != nil {
		if os.IsNotExist(err) {
			raw = nil
		} else {
			return "", err
		}
	}
	view := SelectRelevantCanon(string(raw), briefBody, actors, maxChars)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(runDir, "canon_relevant.md")
	if err := os.WriteFile(path, []byte(view), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// DefaultCanonBudget is the char threshold under which canon is passed
// through verbatim. ~8 KB ≈ 2k tokens — comfortably small enough that
// trimming buys nothing. Above it, filtering kicks in.
const DefaultCanonBudget = 8000
