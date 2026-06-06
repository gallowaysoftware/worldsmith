package world

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Brief planning helpers — for the `brief` generator, which proposes
// the next installment's brief.md from where the story stands. These
// answer "which number am I drafting?" and "which prior brief should I
// show the model as a format exemplar?".

// briefNumbers reads briefs/ once and returns the set of existing brief numbers
// (parsed from NNN.md filenames). A single ReadDir replaces the old up-to-999
// os.Stat loop. Missing dir → empty set, nil error.
func briefNumbers(l Layout) (map[int]bool, error) {
	entries, err := os.ReadDir(l.BriefsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return map[int]bool{}, nil
		}
		return nil, err
	}
	nums := make(map[int]bool, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(name, ".md"))
		if err != nil || n < 1 {
			continue
		}
		nums[n] = true
	}
	return nums, nil
}

// NextBriefNumber returns the smallest 1-indexed installment number
// that has no brief file yet — the next brief to draft. Fills gaps:
// if briefs 001 and 003 exist, it returns 002.
func NextBriefNumber(l Layout) (int, error) {
	nums, err := briefNumbers(l)
	if err != nil {
		return 0, err
	}
	for n := 1; n <= 999; n++ {
		if !nums[n] {
			return n, nil
		}
	}
	return 0, fmt.Errorf("more than 999 briefs — bail out")
}

// LatestBriefNumber returns the highest-numbered existing brief, or 0
// when no briefs exist. The brief generator shows this one to the
// model as a house-style + continuity exemplar.
func LatestBriefNumber(l Layout) int {
	nums, err := briefNumbers(l)
	if err != nil {
		return 0
	}
	last := 0
	for n := range nums {
		if n > last {
			last = n
		}
	}
	return last
}
