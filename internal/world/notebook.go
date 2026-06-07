package world

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The NOTEBOOK is the world's private author layer: the secrets, the
// where-it's-going, the deep character interiority and faction true-agendas
// that the author knows but readers have not been shown. It is distinct from:
//
//   - world.md  — the published bible (human-authored; the LLM NEVER edits it);
//   - canon.md  — facts established by published installments (what readers know);
//   - notebook/ — what the author knows but hasn't revealed (this file's domain).
//
// The expand pipeline proposes notebook dossiers into a STAGING area
// (.expand/<stamp>/); the human reviews and accepts (merge into notebook/, with
// a backup of any overwritten dossier) or discards (drop the staged file).
// Nothing the LLM produces lands in a world non-reversibly.

func (l Layout) NotebookDir() string { return filepath.Join(l.Root, "notebook") }
func (l Layout) NotebookFile(slug string) string {
	return filepath.Join(l.NotebookDir(), slug+".md")
}
func (l Layout) ExpandDir() string { return filepath.Join(l.Root, ".expand") }
func (l Layout) ExpandStagingDir(stamp string) string {
	return filepath.Join(l.ExpandDir(), stamp)
}
func (l Layout) NotebookBackupDir(stamp string) string {
	return filepath.Join(l.NotebookDir(), ".backups", stamp)
}

// Dossier is one notebook thread on disk.
type Dossier struct {
	Slug  string // filename stem
	Title string // parsed from the "# Thread: <Title>" H1, else the slug
	Path  string
	Stamp string // staging timestamp (empty for accepted notebook dossiers)
}

// dossierTitle pulls a human title from a dossier's first heading.
func dossierTitle(path, fallback string) string {
	f, err := os.Open(path)
	if err != nil {
		return fallback
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		// Only a Markdown ATX heading names the dossier. A first line that
		// is a blockquote, rule, or plain paragraph is content, not a title —
		// the old `TrimLeft(line, "# ")` cutset mangled any line that merely
		// began with '#' or a space. Anything else falls back to the slug.
		if !strings.HasPrefix(line, "#") {
			return fallback
		}
		title := strings.TrimSpace(strings.TrimLeft(line, "#"))
		title = strings.TrimSpace(strings.TrimPrefix(title, "Thread:"))
		if title == "" {
			return fallback
		}
		return title
	}
	return fallback
}

// ListDossiers returns the accepted notebook dossiers, sorted by slug.
func ListDossiers(l Layout) ([]Dossier, error) {
	entries, err := os.ReadDir(l.NotebookDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Dossier
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		p := l.NotebookFile(slug)
		out = append(out, Dossier{Slug: slug, Title: dossierTitle(p, slug), Path: p})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out, nil
}

// AssembleNotebook concatenates every accepted dossier into one string — the
// author's full private knowledge, for context during expansion and (later)
// content generation. Returns "" when the notebook is empty.
func AssembleNotebook(l Layout) (string, error) {
	dossiers, err := ListDossiers(l)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, d := range dossiers {
		body, err := os.ReadFile(d.Path)
		if err != nil {
			return "", err
		}
		b.Write(body)
		b.WriteString("\n\n")
	}
	return b.String(), nil
}

// WriteAssembledNotebook materialises AssembleNotebook into runDir/notebook.md
// (or an empty file) and returns the path — the shape pipelines expect as input.
func WriteAssembledNotebook(l Layout, runDir string) (string, error) {
	body, err := AssembleNotebook(l)
	if err != nil {
		return "", err
	}
	path := filepath.Join(runDir, "notebook.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ListStaged returns proposed dossiers across all .expand/<stamp>/ staging dirs,
// newest stamp first.
func ListStaged(l Layout) ([]Dossier, error) {
	stamps, err := os.ReadDir(l.ExpandDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Dossier
	for i := len(stamps) - 1; i >= 0; i-- { // newest first (names sort lexically by timestamp)
		e := stamps[i]
		if !e.IsDir() {
			continue
		}
		stamp := e.Name()
		dir := l.ExpandStagingDir(stamp)
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
				continue
			}
			slug := strings.TrimSuffix(f.Name(), ".md")
			p := filepath.Join(dir, f.Name())
			out = append(out, Dossier{Slug: slug, Title: dossierTitle(p, slug), Path: p, Stamp: stamp})
		}
	}
	return out, nil
}

// AcceptStaged merges a staged dossier into notebook/, backing up any existing
// dossier of the same slug first (so an accept is reversible), then removes the
// staged file. backupStamp groups all backups from one review session.
func AcceptStaged(l Layout, d Dossier, backupStamp string) error {
	if err := os.MkdirAll(l.NotebookDir(), 0o755); err != nil {
		return err
	}
	dst := l.NotebookFile(d.Slug)
	if _, err := os.Stat(dst); err == nil {
		bdir := l.NotebookBackupDir(backupStamp)
		if err := os.MkdirAll(bdir, 0o755); err != nil {
			return err
		}
		if err := CopyFile(dst, filepath.Join(bdir, d.Slug+".md")); err != nil {
			return fmt.Errorf("back up existing dossier: %w", err)
		}
	}
	if err := CopyFile(d.Path, dst); err != nil {
		return err
	}
	return os.Remove(d.Path)
}

// DiscardStaged drops a staged dossier without touching the notebook.
func DiscardStaged(d Dossier) error { return os.Remove(d.Path) }
