package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountFindingsJSON_FenceAndCounts(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
		want map[string]int
	}{
		{
			name: "plain json",
			raw:  `{"findings":[{"severity":"LEAK"},{"severity":"WATCH"}]}`,
			ok:   true,
			want: map[string]int{"LEAK": 1, "WATCH": 1},
		},
		{
			name: "fenced json (the OutputFormatJSON wrapper bug)",
			raw:  "```json\n{\"findings\":[{\"severity\":\"BREAKING\"},{\"severity\":\"BREAKING\"},{\"severity\":\"MINOR\"}]}\n```",
			ok:   true,
			want: map[string]int{"BREAKING": 2, "MINOR": 1},
		},
		{
			name: "empty findings",
			raw:  `{"findings": []}`,
			ok:   true,
			want: map[string]int{},
		},
		{
			name: "lowercase severity normalised",
			raw:  `{"findings":[{"severity":"leak"}]}`,
			ok:   true,
			want: map[string]int{"LEAK": 1},
		},
		{
			name: "legacy markdown is not json",
			raw:  "# Fog report\n\n**Verdict:** 1 leak, 0 watch\n",
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := countFindingsJSON([]byte(tc.raw))
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("count[%s] = %d, want %d (got %v)", k, got[k], v, got)
				}
			}
			if len(got) != len(tc.want) {
				t.Errorf("count map = %v, want %v", got, tc.want)
			}
		})
	}
}

// A fenced continuity report must score by its findings, not fall through to a
// false "clean" 100 (the v9 bug).
func TestContinuityScoreResult_FencedJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "continuity_report.md")
	fenced := "```json\n{\"findings\":[{\"severity\":\"BREAKING\",\"category\":\"World rules\",\"span\":\"x\",\"conflict\":\"y\",\"fix\":\"z\"}]}\n```"
	if err := os.WriteFile(p, []byte(fenced), 0o644); err != nil {
		t.Fatal(err)
	}
	r := ContinuityScoreResult(p)
	if r.Score == 100 {
		t.Fatalf("fenced report with a BREAKING finding scored 100 (false clean); got summary %q", r.Summary)
	}
	if r.Score != 70 { // 100 - 30*1
		t.Errorf("score = %d, want 70 (one breaking)", r.Score)
	}
}
