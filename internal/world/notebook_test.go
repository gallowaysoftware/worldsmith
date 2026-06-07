package world

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stage(t *testing.T, l Layout, stamp, slug, body string) Dossier {
	t.Helper()
	p := filepath.Join(l.ExpandStagingDir(stamp), slug+".md")
	writeFile(t, p, body)
	return Dossier{Slug: slug, Title: dossierTitle(p, slug), Path: p, Stamp: stamp}
}

func TestDossierTitle(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	d := stage(t, l, "2026-01-01T00-00-00", "the-vault", "# Thread: The Vault\n\n> what it is\n")
	if d.Title != "The Vault" {
		t.Errorf("title parse: got %q want %q", d.Title, "The Vault")
	}
	// A first line that is not a heading is content, not a title — fall back
	// to the slug rather than promoting the paragraph to a title.
	d2 := stage(t, l, "2026-01-01T00-00-00", "no-heading", "just text, no heading\n")
	if d2.Title != "no-heading" {
		t.Errorf("non-heading first line should fall back to slug: got %q", d2.Title)
	}
	// A blockquote first line must not be mangled into a title.
	d3 := stage(t, l, "2026-01-01T00-00-00", "quote-first", "> a quoted opening\n")
	if d3.Title != "quote-first" {
		t.Errorf("blockquote first line should fall back to slug: got %q", d3.Title)
	}
	// A plain heading without the "Thread:" prefix keeps its text.
	d4 := stage(t, l, "2026-01-01T00-00-00", "plain-heading", "## The Sunken Archive\n")
	if d4.Title != "The Sunken Archive" {
		t.Errorf("plain heading should drop hashes only: got %q", d4.Title)
	}
	// A heading that is only hashes/whitespace falls back to the slug.
	d5 := stage(t, l, "2026-01-01T00-00-00", "empty-heading", "###\n")
	if d5.Title != "empty-heading" {
		t.Errorf("empty heading should fall back to slug: got %q", d5.Title)
	}
}

func TestAcceptStagedMovesAndBacksUp(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	// Pre-existing accepted dossier with the same slug.
	writeFile(t, l.NotebookFile("the-vault"), "# Thread: The Vault\n\noriginal content\n")
	d := stage(t, l, "2026-05-30T10-00-00", "the-vault", "# Thread: The Vault\n\nNEW content\n")

	if err := AcceptStaged(l, d, "backupstamp"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	// Notebook now holds the new content.
	got, _ := os.ReadFile(l.NotebookFile("the-vault"))
	if string(got) != "# Thread: The Vault\n\nNEW content\n" {
		t.Errorf("notebook not updated: %q", got)
	}
	// The old content was backed up (reversible).
	bak, err := os.ReadFile(filepath.Join(l.NotebookBackupDir("backupstamp"), "the-vault.md"))
	if err != nil {
		t.Fatalf("expected backup: %v", err)
	}
	if string(bak) != "# Thread: The Vault\n\noriginal content\n" {
		t.Errorf("backup wrong: %q", bak)
	}
	// Staged file consumed.
	if _, err := os.Stat(d.Path); !os.IsNotExist(err) {
		t.Errorf("staged file should be removed after accept")
	}
}

func TestAcceptStagedNewNoBackup(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	d := stage(t, l, "2026-05-30T10-00-00", "the-net", "# Thread: The Net\n\nfresh\n")
	if err := AcceptStaged(l, d, "bk"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, err := os.Stat(l.NotebookFile("the-net")); err != nil {
		t.Errorf("dossier not written: %v", err)
	}
	// No prior file → no backup dir created.
	if _, err := os.Stat(l.NotebookBackupDir("bk")); !os.IsNotExist(err) {
		t.Errorf("no backup expected for a brand-new dossier")
	}
}

func TestDiscardStagedLeavesNotebookUntouched(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	writeFile(t, l.NotebookFile("the-vesh"), "# Thread: The Vesh\n\nkeep me\n")
	d := stage(t, l, "2026-05-30T10-00-00", "the-vesh", "# Thread: The Vesh\n\nrejected proposal\n")
	if err := DiscardStaged(d); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(d.Path); !os.IsNotExist(err) {
		t.Errorf("staged file should be gone")
	}
	got, _ := os.ReadFile(l.NotebookFile("the-vesh"))
	if string(got) != "# Thread: The Vesh\n\nkeep me\n" {
		t.Errorf("discard must not touch the accepted notebook: %q", got)
	}
}

func TestListStagedAndAssemble(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	stage(t, l, "2026-05-30T09-00-00", "alpha", "# Thread: Alpha\n\na\n")
	stage(t, l, "2026-05-30T10-00-00", "beta", "# Thread: Beta\n\nb\n")
	staged, err := ListStaged(l)
	if err != nil {
		t.Fatal(err)
	}
	if len(staged) != 2 {
		t.Fatalf("want 2 staged, got %d", len(staged))
	}
	// Newest stamp first.
	if staged[0].Slug != "beta" {
		t.Errorf("expected newest-first ordering, got %q first", staged[0].Slug)
	}

	// Accept both, then the assembled notebook contains both bodies.
	for _, d := range staged {
		if err := AcceptStaged(l, d, "bk"); err != nil {
			t.Fatal(err)
		}
	}
	asm, err := AssembleNotebook(l)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(asm, "Alpha") || !contains(asm, "Beta") {
		t.Errorf("assembled notebook missing dossiers: %q", asm)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
