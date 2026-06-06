package pipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// renderEnumerateSegments executes enumerateSegmentsTmpl with a funcmap that
// mirrors vamp's built-in template helpers, so the test exercises the SAME
// template string the story pipeline ships — not a re-implementation. It
// returns the parsed segments.
//
// This is the regression guard for the dialogue-voice bug: characters.json has
// no `slug` field, so keying segment voices on a `slug` lookup matched nothing
// and every line fell back to the narrator. The fix slugifies the character
// NAME on both the cast side and the showrunner's `host`, so a character's
// configured voice actually reaches its segments.
func renderEnumerateSegments(t *testing.T, charactersJSON, showrunnerJSON, paragraphsJSON, narrator string) []map[string]any {
	t.Helper()

	dir := t.TempDir()
	charsPath := filepath.Join(dir, "characters.json")
	if err := os.WriteFile(charsPath, []byte(charactersJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	funcs := template.FuncMap{
		"slugify":        testSlugify,
		"trim":           strings.TrimSpace,
		"readFile":       func(p string) string { b, _ := os.ReadFile(p); return string(b) },
		"parseJSON":      func(s string) any { var v any; _ = json.Unmarshal([]byte(s), &v); return v },
		"toJSON":         func(v any) string { b, _ := json.Marshal(v); return string(b) },
		"addInt":         func(a, b int) int { return a + b },
		"splitSentences": testSplitSentences,
	}
	tmpl, err := template.New("enum").Funcs(funcs).Parse(enumerateSegmentsTmpl)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	data := map[string]any{
		"inputs": map[string]any{
			"characters_file": charsPath,
			"narrator_voice":  narrator,
		},
		"stages": map[string]any{
			"number_paragraphs": map[string]any{"output": paragraphsJSON},
			"showrunner":        map[string]any{"output": showrunnerJSON},
		},
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("execute template: %v", err)
	}
	var out struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("template produced invalid JSON: %v\n%s", err, buf.String())
	}
	return out.Items
}

func TestEnumerateSegmentsRoutesCharacterVoice(t *testing.T) {
	characters := `{"characters": [
		{"name": "Tova Marsh", "voice_id": "af_bella"},
		{"name": "Captain Voss", "voice_id": "am_michael"}
	]}`
	// The showrunner emits the character NAME as host (verbatim), exactly as
	// the updated prompt instructs; idx 0 is narration.
	showrunner := `{"segments": [
		{"idx": 0, "host": "narrator"},
		{"idx": 1, "host": "Tova Marsh"},
		{"idx": 2, "host": "Captain Voss"}
	]}`
	paragraphs := `{"items": [
		{"idx": 0, "text": "The harbor lay still under a bruised sky."},
		{"idx": 1, "text": "I will not sign it."},
		{"idx": 2, "text": "Then we are both finished here."}
	]}`

	segs := renderEnumerateSegments(t, characters, showrunner, paragraphs, "am_fenrir")
	if len(segs) != 3 {
		t.Fatalf("got %d segments, want 3", len(segs))
	}

	want := []string{"am_fenrir", "af_bella", "am_michael"}
	for i, w := range want {
		if got := segs[i]["voice_id"]; got != w {
			t.Errorf("segment %d voice_id = %v, want %q (host=%v)", i, got, w, segs[i]["host"])
		}
	}
}

func TestEnumerateSegmentsUnknownHostFallsBackToNarrator(t *testing.T) {
	characters := `{"characters": [{"name": "Tova Marsh", "voice_id": "af_bella"}]}`
	// A host not in the cast (a character the bible dropped, or a model slip)
	// must fall back to the narrator rather than emit an empty voice.
	showrunner := `{"segments": [{"idx": 0, "host": "Ghost Who Was Cut"}]}`
	paragraphs := `{"items": [{"idx": 0, "text": "Someone spoke from the dark."}]}`

	segs := renderEnumerateSegments(t, characters, showrunner, paragraphs, "am_fenrir")
	if len(segs) != 1 {
		t.Fatalf("got %d segments, want 1", len(segs))
	}
	if got := segs[0]["voice_id"]; got != "am_fenrir" {
		t.Errorf("unknown host voice_id = %v, want narrator fallback am_fenrir", got)
	}
}

// testSlugify mirrors vamp's slugify (internal/vamp/exec.go): lowercase ASCII
// alphanumerics, every other run collapses to a single hyphen, trimmed, capped
// at 60 bytes. Kept in sync so this test exercises the real matching behavior.
func testSlugify(v any) string {
	in := strings.ToLower(toStr(v))
	var b strings.Builder
	prevHyphen := false
	for _, r := range in {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
			continue
		}
		if !prevHyphen && b.Len() > 0 {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-")
	}
	return s
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// testSplitSentences mirrors vamp's splitSentences for the short, single-chunk
// inputs this test uses: a string under maxChars returns a one-element JSON
// array. (The full greedy-packing path isn't exercised here.)
func testSplitSentences(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	b, _ := json.Marshal([]string{text})
	return string(b)
}
