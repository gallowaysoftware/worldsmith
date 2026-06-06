package world

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBriefReveals(t *testing.T) {
	raw := `---
pov_region: pharos
on_stage_actors: [ila, doran]
reveals:
  - the-vault
  - "the losses in the dark were captures"
---
The brief body.
`
	front, body, err := splitFrontmatter([]byte(raw))
	if err != nil {
		t.Fatalf("splitFrontmatter: %v", err)
	}
	if len(front.Reveals) != 2 {
		t.Fatalf("Reveals = %v, want 2 entries", front.Reveals)
	}
	if front.Reveals[0] != "the-vault" || front.Reveals[1] != "the losses in the dark were captures" {
		t.Errorf("Reveals = %#v", front.Reveals)
	}
	if strings.TrimSpace(body) != "The brief body." {
		t.Errorf("body = %q", body)
	}
}

func TestParseBriefNoReveals(t *testing.T) {
	front, _, err := splitFrontmatter([]byte("---\npov_region: x\n---\nbody\n"))
	if err != nil {
		t.Fatalf("splitFrontmatter: %v", err)
	}
	if len(front.Reveals) != 0 {
		t.Errorf("Reveals = %v, want empty", front.Reveals)
	}
}

func TestWriteLicensedReveals(t *testing.T) {
	dir := t.TempDir()

	// With licensed reveals: each appears as a bullet, no "None" sentinel.
	p, err := WriteLicensedReveals(dir, []string{"the-vault", "  ", "the slow approach"})
	if err != nil {
		t.Fatalf("WriteLicensedReveals: %v", err)
	}
	if p != filepath.Join(dir, "licensed_reveals.md") {
		t.Errorf("path = %q", p)
	}
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "- the-vault") || !strings.Contains(s, "- the slow approach") {
		t.Errorf("missing licensed bullets:\n%s", s)
	}
	if strings.Contains(s, "None.") {
		t.Errorf("should not carry the none sentinel when reveals present:\n%s", s)
	}
	// The blank entry is skipped, so only two bullets.
	if got := strings.Count(s, "\n- "); got != 2 {
		t.Errorf("bullet count = %d, want 2:\n%s", got, s)
	}

	// Empty: the "none" sentinel, no bullets.
	p2, err := WriteLicensedReveals(dir, nil)
	if err != nil {
		t.Fatalf("WriteLicensedReveals(nil): %v", err)
	}
	body2, _ := os.ReadFile(p2)
	if !strings.Contains(string(body2), "None.") {
		t.Errorf("empty reveals should carry the none sentinel:\n%s", body2)
	}
}
