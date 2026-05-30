package world

import (
	"fmt"
	"os"
)

// Brief planning helpers — for the `brief` generator, which proposes
// the next installment's brief.md from where the story stands. These
// answer "which number am I drafting?" and "which prior brief should I
// show the model as a format exemplar?".

// NextBriefNumber returns the smallest 1-indexed installment number
// that has no brief file yet — the next brief to draft. Fills gaps:
// if briefs 001 and 003 exist, it returns 002.
func NextBriefNumber(l Layout) (int, error) {
	for n := 1; n <= 999; n++ {
		_, err := os.Stat(l.BriefFile(n))
		if err == nil {
			continue
		}
		if os.IsNotExist(err) {
			return n, nil
		}
		return 0, err
	}
	return 0, fmt.Errorf("more than 999 briefs — bail out")
}

// LatestBriefNumber returns the highest-numbered existing brief, or 0
// when no briefs exist. The brief generator shows this one to the
// model as a house-style + continuity exemplar.
func LatestBriefNumber(l Layout) int {
	last := 0
	for n := 1; n <= 999; n++ {
		if _, err := os.Stat(l.BriefFile(n)); err == nil {
			last = n
		}
	}
	return last
}
