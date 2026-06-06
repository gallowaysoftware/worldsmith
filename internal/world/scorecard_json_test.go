package world

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStripJSONFence(t *testing.T) {
	cases := []struct{ in, want string }{
		{"{\"a\":1}", `{"a":1}`},
		{"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"```\n{\"a\":1}\n```", `{"a":1}`},
		{"   {\"a\":1}   ", `{"a":1}`},
		{"not json at all", "not json at all"},
	}
	for _, c := range cases {
		if got := string(StripJSONFence([]byte(c.in))); got != c.want {
			t.Errorf("StripJSONFence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

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
		{
			name: "json object without a findings key is not a findings doc",
			raw:  `{"error":"model refused","reason":"context too long"}`,
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

// A self-negating finding (one whose conflict text talks itself out of being a finding)
// must be dropped by the scorecard, exactly as the verify-loop's verdictCount drops it,
// so the shipped score tracks the same target the loop optimised against.
func TestCountFindingsJSON_DropsSelfNegating(t *testing.T) {
	raw := []byte(`{"findings":[
		{"severity":"BREAKING","conflict":"contradicts the bible: the gate was sealed"},
		{"severity":"BREAKING","conflict":"On reflection this is consistent. No contradiction."}
	]}`)
	got, ok := countFindingsJSON(raw)
	if !ok {
		t.Fatal("expected a parsed findings document")
	}
	if got["BREAKING"] != 1 {
		t.Errorf("BREAKING = %d, want 1 (self-negating finding dropped); got %v", got["BREAKING"], got)
	}
}

// A JSON object that isn't a findings document (e.g. an error blob) must not score a
// false clean 100 — that would let a broken checker run masquerade as a perfect one.
func TestScoreResult_NonFindingsJSONIsNotClean(t *testing.T) {
	dir := t.TempDir()
	blob := []byte(`{"error":"model produced no findings array"}`)

	cont := filepath.Join(dir, "continuity_report.md")
	if err := os.WriteFile(cont, blob, 0o644); err != nil {
		t.Fatal(err)
	}
	if r := ContinuityScoreResult(cont); r.Score == 100 {
		t.Errorf("non-findings JSON scored a false clean 100 for continuity: %q", r.Summary)
	}

	fog := filepath.Join(dir, "fog_report.md")
	if err := os.WriteFile(fog, blob, 0o644); err != nil {
		t.Fatal(err)
	}
	if r := FogScoreResult(fog); r.Score == 100 {
		t.Errorf("non-findings JSON scored a false clean 100 for fog: %q", r.Summary)
	}
}
