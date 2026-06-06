package world

import (
	"os"
	"strings"
	"testing"
)

// writeCanonDelta drops a canon_delta.md for installment n so AppendCanonDelta can fold
// it in.
func writeCanonDelta(t *testing.T, l Layout, n int, body string) {
	t.Helper()
	if err := os.MkdirAll(l.InstallmentDir(n), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(l.InstallmentFile(n, "canon_delta.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readCanon(t *testing.T, l Layout) string {
	t.Helper()
	b, err := os.ReadFile(l.CanonFile())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAppendCanonDelta_BuildsOrderedSections(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	for _, n := range []int{1, 2, 3} {
		writeCanonDelta(t, l, n, "- fact from installment "+itoa(n))
		if err := AppendCanonDelta(l, n); err != nil {
			t.Fatalf("AppendCanonDelta(%d): %v", n, err)
		}
	}
	got := readCanon(t, l)
	for _, want := range []string{
		"## From installment 1", "## From installment 2", "## From installment 3",
		"fact from installment 1", "fact from installment 3",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("canon missing %q:\n%s", want, got)
		}
	}
	// Ordered 1 < 2 < 3.
	if !ordered(got, "installment 1", "installment 2", "installment 3") {
		t.Errorf("sections out of order: %s", got)
	}
}

// The core fix for the --installment N truncation bug: regenerating an EARLIER
// installment must not drop the canon of LATER ones.
func TestAppendCanonDelta_RegenEarlierKeepsLater(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	for _, n := range []int{1, 2, 3} {
		writeCanonDelta(t, l, n, "- original fact "+itoa(n))
		if err := AppendCanonDelta(l, n); err != nil {
			t.Fatal(err)
		}
	}

	// Regenerate installment 2: pre-run truncate (drops only 2), then re-append a
	// changed delta.
	if err := TruncateCanonFrom(l, 2); err != nil {
		t.Fatal(err)
	}
	after := readCanon(t, l)
	if strings.Contains(after, "original fact 2") {
		t.Errorf("TruncateCanonFrom(2) should have removed installment 2's section:\n%s", after)
	}
	if !strings.Contains(after, "original fact 3") {
		t.Errorf("TruncateCanonFrom(2) must keep installment 3's section:\n%s", after)
	}

	writeCanonDelta(t, l, 2, "- revised fact 2")
	if err := AppendCanonDelta(l, 2); err != nil {
		t.Fatal(err)
	}
	got := readCanon(t, l)
	if !strings.Contains(got, "revised fact 2") {
		t.Errorf("revised installment 2 not folded in:\n%s", got)
	}
	if !strings.Contains(got, "original fact 3") {
		t.Errorf("installment 3 canon was dropped by regenerating 2 — the bug:\n%s", got)
	}
	if !strings.Contains(got, "original fact 1") {
		t.Errorf("installment 1 canon was dropped:\n%s", got)
	}
	// 2's revised section must sit between 1 and 3.
	if !ordered(got, "original fact 1", "revised fact 2", "original fact 3") {
		t.Errorf("re-inserted section out of order:\n%s", got)
	}
}

func TestCanonAsOf_ExcludesSelfAndFuture(t *testing.T) {
	canon := "preamble line\n\n" +
		"## From installment 1\n\n- a\n\n" +
		"## From installment 2\n\n- b\n\n" +
		"## From installment 3\n\n- c\n"
	got := CanonAsOf(canon, 2)
	if !strings.Contains(got, "- a") {
		t.Errorf("as-of 2 should include installment 1:\n%s", got)
	}
	if strings.Contains(got, "- b") {
		t.Errorf("as-of 2 should exclude installment 2's own section:\n%s", got)
	}
	if strings.Contains(got, "- c") {
		t.Errorf("as-of 2 should exclude the future installment 3:\n%s", got)
	}
	if !strings.Contains(got, "preamble line") {
		t.Errorf("as-of should keep the preamble:\n%s", got)
	}
}

func TestCanonAsOf_LegacyFlatCanonVerbatim(t *testing.T) {
	flat := "- some fact with no installment header\n- another\n"
	if got := CanonAsOf(flat, 5); got != flat {
		t.Errorf("flat canon should pass through verbatim; got %q", got)
	}
}

// ordered reports whether each marker first appears strictly after the previous one in s.
func ordered(s string, markers ...string) bool {
	prev := -1
	for _, m := range markers {
		i := strings.Index(s, m)
		if i <= prev {
			return false
		}
		prev = i
	}
	return true
}

// itoa is a tiny local helper so the test file doesn't pull in strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
