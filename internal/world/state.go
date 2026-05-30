// Package world manages the on-disk state for a worldsmith world:
// the directory layout, the canonical file paths, and helpers that
// read/write/list world state. The CLI binary owns the flow; this
// package owns the bookkeeping.
//
// Layout mirrors fake-crime's series state with three differences:
//  1. world bible is user-authored, not LLM-generated;
//  2. per-installment briefs (briefs/NNN.md) replace arc.json's
//     fixed beat sheet as the primary driver;
//  3. chapters within a single installment are tracked (for novels)
//     via installments/NNN/chapters/.
package world

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Layout returns the canonical file paths for a world. Centralised
// so every caller agrees on where things live.
type Layout struct {
	Root string
}

// DefaultRoot returns $XDG_STATE_HOME/worldsmith, or
// ~/.local/state/worldsmith as fallback.
func DefaultRoot() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "worldsmith")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "worldsmith"
	}
	return filepath.Join(home, ".local", "state", "worldsmith")
}

// Open returns a Layout for the given world slug, creating the
// directory structure on first call. Idempotent on re-runs — the
// MkdirAll calls don't clobber existing content.
func Open(slug string) (Layout, error) {
	root := filepath.Join(DefaultRoot(), slug)
	for _, sub := range []string{"briefs", "installments", "scenes"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			return Layout{}, err
		}
	}
	return Layout{Root: root}, nil
}

func (l Layout) WorldFile() string       { return filepath.Join(l.Root, "world.md") }
func (l Layout) CharactersFile() string  { return filepath.Join(l.Root, "characters.json") }
func (l Layout) ArcFile() string         { return filepath.Join(l.Root, "arc.json") }
func (l Layout) CanonFile() string       { return filepath.Join(l.Root, "canon.md") }
func (l Layout) BriefsDir() string       { return filepath.Join(l.Root, "briefs") }
func (l Layout) InstallmentsDir() string { return filepath.Join(l.Root, "installments") }
func (l Layout) ScenesDir() string       { return filepath.Join(l.Root, "scenes") }

// SceneDir returns the per-scene output dir (1-indexed).
func (l Layout) SceneDir(n int) string {
	return filepath.Join(l.ScenesDir(), fmt.Sprintf("%03d", n))
}

// SceneFile is a path helper inside a scene dir.
func (l Layout) SceneFile(n int, name string) string {
	return filepath.Join(l.SceneDir(n), name)
}

// NextScene returns the smallest 1-indexed scene number without a finished
// final.mp4.
func NextScene(l Layout) (int, error) {
	entries, err := os.ReadDir(l.ScenesDir())
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	done := map[int]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(e.Name(), "%d", &n); err != nil {
			continue
		}
		if _, err := os.Stat(l.SceneFile(n, "final.mp4")); err == nil {
			done[n] = true
		}
	}
	for n := 1; n <= 999; n++ {
		if !done[n] {
			return n, nil
		}
	}
	return 0, fmt.Errorf("more than 999 scenes — bail out")
}

// BriefFile returns the brief path for installment n (1-indexed).
func (l Layout) BriefFile(n int) string {
	return filepath.Join(l.BriefsDir(), fmt.Sprintf("%03d.md", n))
}

// InstallmentDir returns the per-installment output dir.
func (l Layout) InstallmentDir(n int) string {
	return filepath.Join(l.InstallmentsDir(), fmt.Sprintf("%03d", n))
}

// InstallmentFile is a path helper inside an installment dir.
func (l Layout) InstallmentFile(n int, name string) string {
	return filepath.Join(l.InstallmentDir(n), name)
}

// List returns the slug names of every world under DefaultRoot,
// sorted lexically.
func List() ([]string, error) {
	entries, err := os.ReadDir(DefaultRoot())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// CompletedInstallments lists installment numbers (1-indexed) that
// have a finished episode.m4b on disk.
func CompletedInstallments(l Layout) ([]int, error) {
	entries, err := os.ReadDir(l.InstallmentsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(e.Name(), "%d", &n); err != nil {
			continue
		}
		if _, err := os.Stat(l.InstallmentFile(n, "episode.m4b")); err == nil {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out, nil
}

// NextInstallment returns the smallest 1-indexed number without a
// completed episode.m4b.
func NextInstallment(l Layout) (int, error) {
	done, err := CompletedInstallments(l)
	if err != nil {
		return 0, err
	}
	seen := map[int]bool{}
	for _, n := range done {
		seen[n] = true
	}
	for n := 1; n <= 999; n++ {
		if !seen[n] {
			return n, nil
		}
	}
	return 0, fmt.Errorf("more than 999 installments — bail out")
}

// EnsureCanonFile creates an empty canon.md when none exists. Returns
// the path either way. The episode pipeline's canon_delta append
// path expects the file to exist before it runs.
func EnsureCanonFile(l Layout) (string, error) {
	p := l.CanonFile()
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(p, []byte(""), 0o644); err != nil {
				return "", err
			}
		} else {
			return "", err
		}
	}
	return p, nil
}

// AssemblePriors concatenates every summary.md from installments
// 1..n-1 into a single string the next installment's writer reads
// as priors_file. Empty string when n=1.
func AssemblePriors(l Layout, upToButNotIncluding int) (string, error) {
	var b strings.Builder
	for i := 1; i < upToButNotIncluding; i++ {
		raw, err := os.ReadFile(l.InstallmentFile(i, "summary.md"))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		fmt.Fprintf(&b, "## Installment %d\n\n%s\n\n", i, strings.TrimSpace(string(raw)))
	}
	return b.String(), nil
}

// EnsurePriorsFile writes the assembled priors into the per-
// installment run dir so a vamp prompt can readFile it.
func EnsurePriorsFile(l Layout, runDir string, upToButNotIncluding int) (string, error) {
	content, err := AssemblePriors(l, upToButNotIncluding)
	if err != nil {
		return "", err
	}
	path := filepath.Join(runDir, "priors.md")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// AppendCanonDelta folds a freshly-finished installment's canon
// delta into the running canon.md. Idempotent — checks for the
// per-installment header before appending so re-running on the
// same installment doesn't double up.
func AppendCanonDelta(l Layout, n int) error {
	delta, err := os.ReadFile(l.InstallmentFile(n, "canon_delta.md"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	header := fmt.Sprintf("\n## From installment %d\n\n", n)
	existing, _ := os.ReadFile(l.CanonFile())
	if strings.Contains(string(existing), header) {
		return nil
	}
	f, err := os.OpenFile(l.CanonFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(header); err != nil {
		return err
	}
	_, err = f.Write(delta)
	return err
}

// CharactersDoc is the on-disk shape of characters.json. Loose
// schema — the file gets piped into prompts as raw JSON, the engine
// doesn't strictly type the cast.
type CharactersDoc struct {
	Characters []map[string]any `json:"characters"`
}

// LoadCharacters reads + parses the characters.json file. Returns
// an empty doc + nil error when the file doesn't exist so a brand-
// new world without a cast doesn't break the story pipeline.
func LoadCharacters(l Layout) (CharactersDoc, error) {
	raw, err := os.ReadFile(l.CharactersFile())
	if err != nil {
		if os.IsNotExist(err) {
			return CharactersDoc{}, nil
		}
		return CharactersDoc{}, err
	}
	var doc CharactersDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return CharactersDoc{}, fmt.Errorf("parse %s: %w", l.CharactersFile(), err)
	}
	return doc, nil
}

// ScaffoldWorld writes stub files into a fresh world dir. Used by
// `worldsmith init <slug>` so a new user has something to edit
// rather than starting at a blank prompt.
func ScaffoldWorld(l Layout, slug string) error {
	files := map[string]string{
		l.WorldFile():      worldStub(slug),
		l.CharactersFile(): charactersStub(),
		l.CanonFile():      "",
	}
	for path, body := range files {
		// Don't clobber existing user content on re-init.
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	// Drop a stub first brief so the user sees the per-installment
	// driver layer too.
	brief := l.BriefFile(1)
	if _, err := os.Stat(brief); err != nil && os.IsNotExist(err) {
		if err := os.WriteFile(brief, []byte(briefStub(slug)), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", brief, err)
		}
	}
	return nil
}

func worldStub(slug string) string {
	return fmt.Sprintf(`# %s

<!-- WORLD BIBLE — you write this. Everything below is a scaffold to fill in. -->

## Setting

Where? When? What's the feel? One paragraph that anchors a reader who's never heard of this world.

## History

The 2-3 events before any installment that shape the present. Wars, foundings, disasters, discoveries — the things characters can refer to in passing without explanation.

## Factions / forces

Who's pulling at what. Could be governments, guilds, gods, corporations, ideologies. List the 3-5 that matter.

## Tone

The reader's bedside-table feel. "Tolkien-melancholy," "Le Guin-quiet," "Hyperion-baroque," "Cormac McCarthy-spare." A sentence is enough.

## Rules

The hard ones the LLM needs to honor. Magic costs / tech limits / what's impossible.

`, slug)
}

func charactersStub() string {
	return `{
  "characters": [
    {
      "name": "<name>",
      "role": "<one-liner — protagonist, foil, mentor, hidden antagonist, etc.>",
      "voice": "<how they sound on the page — terse, formal, anecdotal>",
      "want": "<what drives them>",
      "knows": ["<load-bearing piece of canon they're a vehicle for>"],
      "arc_hint": "<optional — where they're headed across the series>"
    }
  ]
}
`
}

func briefStub(slug string) string {
	return fmt.Sprintf(`# Installment 1 brief — %s

<!-- Brief = your direction for this specific installment. The
     world bible + characters file stay constant; this is where you
     point the LLM at what THIS story is about. -->

## Hook

One sentence. The reader's reason to start.

## What happens

Three to five bullets. Specific enough that the LLM can extrapolate
prose; loose enough that it has room to surprise you.

  - <event 1>
  - <event 2>
  - <event 3>

## Pov / lens

Whose head are we in? Single-POV or shifting? First, third-limited,
omniscient?

## Constraints

Anything the LLM should NOT do. ("Don't kill X." "No magic system
reveal yet." "Keep the harbor town's name unknown.")
`, slug)
}
