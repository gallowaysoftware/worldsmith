package world

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
)

// ProseMetrics is a deterministic, no-LLM quality read on a finished
// installment's prose. It exists because an LLM judge can't reliably
// see structural degradation (slop-word density, anaphora, the
// "not-X-but-Y" reflex) even when told to look for it — those are
// counting problems, and counting is what code is for. The story
// pipeline writes one of these to metrics.json beside the m4b so a
// run's prose health is a diffable number across installments, not a
// vibe.
//
// None of these are hard gates; they're signal. A high SlopPer1000 or
// a pile of RepeatedOpeners on one installment, when prior ones ran
// clean, is the flag to re-roll or hand-edit.
type ProseMetrics struct {
	Words     int `json:"words"`
	Sentences int `json:"sentences"`

	// SlopHits maps each detected slop term to its count; SlopTotal is
	// their sum and SlopPer1000 normalises it per 1,000 words so the
	// number is comparable across installments of different length.
	SlopHits    map[string]int `json:"slop_hits,omitempty"`
	SlopTotal   int            `json:"slop_total"`
	SlopPer1000 float64        `json:"slop_per_1000"`

	// NotXButY counts the antithesis cadence ("It wasn't anger, but
	// something colder") that generated prose massively over-uses.
	NotXButY int `json:"not_x_but_y"`

	// RepeatedTrigrams are 3-word sequences appearing 3+ times — the
	// fingerprint of a model looping a phrase. RepeatedOpeners are
	// sentence-opening 2-word stems appearing 4+ times — anaphora
	// collapse ("He thought of X. He thought of Y.").
	RepeatedTrigrams []NGramCount `json:"repeated_trigrams,omitempty"`
	RepeatedOpeners  []NGramCount `json:"repeated_openers,omitempty"`
}

// NGramCount is one repeated phrase and how often it occurred.
type NGramCount struct {
	Phrase string `json:"phrase"`
	Count  int    `json:"count"`
}

// SlopTerms is the curated set of words and short phrases that are
// wildly over-represented in LLM fiction relative to human prose.
// Sourced from the public slop-forensics / EQ-Bench slop work and
// from worldsmith's own edit-pass tells. Matched case-insensitively
// on word boundaries. Keep this list conservative: a false positive
// here just inflates a number, but too many turns the signal to noise.
var SlopTerms = []string{
	// over-used verbs / nouns
	"shimmer", "shimmered", "shimmering",
	"glint", "glinted", "glinting",
	"thrum", "thrummed", "thrumming",
	"pulse", "pulsed", "pulsing",
	"cascade", "cascaded", "cascading",
	"tapestry", "woven", "weave",
	"palpable", "testament", "symphony", "kaleidoscope",
	"liminal", "ineffable", "ethereal", "myriad",
	"cacophony", "labyrinthine", "inexorable", "inexorably",
	// portent / vagueness
	"somehow", "something shifted", "for a moment", "in that instant",
	"a kind of", "as if the world",
	// body-as-emotion-readout
	"breath she didn't know", "breath he didn't know",
	"knot in her stomach", "knot in his stomach",
	"jaw tightened", "jaw clenched",
	"shiver down", "behind his eyes", "behind her eyes",
}

var (
	// notXButY matches the antithesis reflex in its common shapes:
	// "not anger but ...", "wasn't a sound but ...", "didn't walk; he
	// drifted" is harder, so we target the explicit not/n't ... but
	// form, which is the dominant tell.
	notXButYRe = regexp.MustCompile(`(?i)(\bnot\b|n't)[^.;!?]{1,60}?\bbut\b`)

	wordRe = regexp.MustCompile(`[\p{L}']+`)
	// sentenceSplitRe splits on terminal punctuation followed by space
	// or end. Good enough for prose metrics; not a linguistics-grade
	// tokenizer.
	sentenceSplitRe = regexp.MustCompile(`[.!?]+[\s"']*`)
)

// WriteProseMetrics reads the prose at storyPath, analyses it, and
// writes the metrics as indented JSON to outPath. Returns the metrics
// so the caller can print a one-line summary. A missing story file is
// not fatal to the caller's flow — it returns the (os.IsNotExist)
// error and lets the caller decide.
func WriteProseMetrics(storyPath, outPath string) (ProseMetrics, error) {
	raw, err := os.ReadFile(storyPath)
	if err != nil {
		return ProseMetrics{}, err
	}
	m := AnalyzeProse(string(raw))
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return m, err
	}
	return m, nil
}

// AnalyzeProse computes ProseMetrics for a block of prose. Safe on
// empty input (returns a zero-ish struct, no panic).
func AnalyzeProse(text string) ProseMetrics {
	lower := strings.ToLower(text)
	words := wordRe.FindAllString(lower, -1)

	m := ProseMetrics{
		Words:    len(words),
		SlopHits: map[string]int{},
	}

	// Slop terms. Multi-word terms are substring-matched on the
	// lowercased text; single words are matched as whole tokens so
	// "pulse" doesn't fire inside "impulse".
	wordSet := map[string]int{}
	for _, w := range words {
		wordSet[w]++
	}
	for _, term := range SlopTerms {
		var c int
		if strings.Contains(term, " ") {
			c = strings.Count(lower, term)
		} else {
			c = wordSet[term]
		}
		if c > 0 {
			m.SlopHits[term] = c
			m.SlopTotal += c
		}
	}
	if m.Words > 0 {
		m.SlopPer1000 = float64(m.SlopTotal) * 1000.0 / float64(m.Words)
	}

	// "not X but Y"
	m.NotXButY = len(notXButYRe.FindAllString(text, -1))

	// Sentences + repeated openers.
	sentences := splitSentencesForMetrics(text)
	m.Sentences = len(sentences)
	openerCounts := map[string]int{}
	for _, s := range sentences {
		toks := wordRe.FindAllString(strings.ToLower(s), -1)
		if len(toks) >= 2 {
			openerCounts[toks[0]+" "+toks[1]]++
		}
	}
	m.RepeatedOpeners = topNGrams(openerCounts, 4, 8)

	// Repeated trigrams across the whole text.
	triCounts := map[string]int{}
	for i := 0; i+2 < len(words); i++ {
		triCounts[words[i]+" "+words[i+1]+" "+words[i+2]]++
	}
	m.RepeatedTrigrams = topNGrams(triCounts, 3, 12)

	return m
}

// OffendingSpan is one sentence flagged for a style rewrite, paired with the
// reason(s) it tripped — fed to the prose-polish pass so the model recasts ONLY
// the offending sentences (length-safe span splice) instead of rewriting the
// whole document (which compresses length).
type OffendingSpan struct {
	Span   string `json:"span"`
	Reason string `json:"reason"`
}

// OffendingSentences returns up to maxSpans verbatim sentence spans that drive
// the prose score down — the same signals AnalyzeProse measures, localised to
// the exact sentences so they can be surgically recast:
//   - a sentence sharing an over-used opening stem (kept the first two of each
//     stem so variety isn't over-corrected; the excess are flagged),
//   - a sentence containing a slop term,
//   - a sentence using the "not X but Y" antithesis cadence.
//
// Spans are the original-case sentence bodies (verbatim substrings of text), so
// the caller can splice replacements back by exact match. Returns nil when the
// prose is already clean.
func OffendingSentences(text string, maxSpans int) []OffendingSpan {
	if maxSpans <= 0 {
		maxSpans = 40
	}
	m := AnalyzeProse(text)
	over := make(map[string]bool, len(m.RepeatedOpeners))
	for _, o := range m.RepeatedOpeners {
		over[o.Phrase] = true
	}
	openerSeen := map[string]int{}
	seenSpan := map[string]bool{}
	var out []OffendingSpan
	for _, s := range splitSentencesForMetrics(text) {
		if len(out) >= maxSpans {
			break
		}
		s = strings.TrimSpace(s)
		// Too short to locate uniquely / safely splice.
		if len(s) < 25 || seenSpan[s] {
			continue
		}
		var reasons []string
		toks := wordRe.FindAllString(strings.ToLower(s), -1)
		if len(toks) >= 2 {
			stem := toks[0] + " " + toks[1]
			if over[stem] {
				openerSeen[stem]++
				if openerSeen[stem] > 2 { // keep the first two; flag the rest
					reasons = append(reasons, `repeated opener "`+stem+`"`)
				}
			}
		}
		ls := strings.ToLower(s)
		for term := range m.SlopHits {
			var hit bool
			if strings.Contains(term, " ") {
				hit = strings.Contains(ls, term)
			} else {
				for _, t := range toks {
					if t == term {
						hit = true
						break
					}
				}
			}
			if hit {
				reasons = append(reasons, `slop "`+term+`"`)
				break
			}
		}
		if notXButYRe.MatchString(s) {
			reasons = append(reasons, "not-X-but-Y cadence")
		}
		if len(reasons) > 0 {
			seenSpan[s] = true
			out = append(out, OffendingSpan{Span: s, Reason: strings.Join(reasons, "; ")})
		}
	}
	return out
}

// splitSentencesForMetrics is a lightweight sentence splitter for the
// metrics pass only. It is deliberately separate from the TTS-side
// splitter (which packs to a char budget); here we just want
// sentence-ish units to read opening stems off.
func splitSentencesForMetrics(text string) []string {
	parts := sentenceSplitRe.Split(text, -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// topNGrams returns phrases occurring at least minCount times, sorted
// by count descending then phrase ascending, capped at limit.
func topNGrams(counts map[string]int, minCount, limit int) []NGramCount {
	var out []NGramCount
	for phrase, c := range counts {
		if c >= minCount {
			out = append(out, NGramCount{Phrase: phrase, Count: c})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Phrase < out[j].Phrase
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
