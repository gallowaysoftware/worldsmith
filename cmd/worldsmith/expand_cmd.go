package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/vibe/contentkit"
	"github.com/gallowaysoftware/vibe/vamp"

	"github.com/gallowaysoftware/worldsmith/internal/pipeline"
	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// expandCommand deepens a world's threads into private notebook dossiers
// (proposals), and `expand review` accepts/discards them. The world bible is
// never edited; nothing lands in the notebook without human review.
func expandCommand() *cobra.Command {
	var (
		slug  string
		seed  string
		count int
	)
	cmd := &cobra.Command{
		Use:   "expand <slug>",
		Short: "Deepen world threads into private notebook dossiers (for review).",
		Long: `expand develops the author's private NOTEBOOK — the secrets, the
where-it's-going, the deep interiority that readers haven't been shown — without
ever touching world.md.

With --seed "<idea>" it develops your idea; without one it auto-selects the
richest unpulled thread(s) from the bible. Each run writes PROPOSED dossiers to a
staging area; accept or discard them with 'worldsmith expand review <slug>'.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required")
			}
			return runExpand(cmd, slug, seed, count)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().StringVar(&seed, "seed", "", "An idea/thread to develop; empty = auto-select.")
	cmd.Flags().IntVar(&count, "count", 1, "How many threads to develop (auto mode).")
	cmd.AddCommand(expandReviewCommand())
	return cmd
}

type threadMeta struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Why   string `json:"why"`
}

func runExpand(cmd *cobra.Command, slug, seed string, count int) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	unlock, err := lockWorld(layout)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Stat(layout.WorldFile()); err != nil {
		return fmt.Errorf("world.md not found at %s — run `worldsmith init %s` first", layout.WorldFile(), slug)
	}
	canonPath, err := world.EnsureCanonFile(layout)
	if err != nil {
		return fmt.Errorf("ensure canon: %w", err)
	}
	if seed != "" {
		count = 1 // a seed develops exactly the one idea
	}
	if count < 1 {
		count = 1
	}

	stamp := time.Now().Format("2006-01-02T15-04-05")
	stagingDir := layout.ExpandStagingDir(stamp)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "world: %s\n", slug)
	if seed != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "developing your seed into a dossier...\n\n")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "auto-selecting and developing %d thread(s)...\n\n", count)
	}

	var chosen []string
	staged := 0
	for i := 0; i < count; i++ {
		genDir := filepath.Join(stagingDir, ".gen", fmt.Sprintf("%03d", i+1))
		if err := os.MkdirAll(genDir, 0o755); err != nil {
			return err
		}
		notebookPath, err := world.WriteAssembledNotebook(layout, genDir)
		if err != nil {
			return fmt.Errorf("assemble notebook: %w", err)
		}
		cfg := pipeline.ExpandConfig{
			WorldFile:      layout.WorldFile(),
			CharactersFile: layout.CharactersFile(),
			CanonFile:      canonPath,
			NotebookFile:   notebookPath,
			Seed:           seed,
			AvoidThreads:   strings.Join(chosen, "\n"),
		}
		root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
			return pipeline.BuildExpand(cfg)
		})
		if err != nil {
			return err
		}
		root.SetArgs([]string{"run", "--run-dir", genDir, "--no-cache"})
		if err := root.ExecuteContext(cmd.Context()); err != nil {
			return fmt.Errorf("expand thread %d: %w", i+1, err)
		}

		var meta threadMeta
		if raw, err := os.ReadFile(filepath.Join(genDir, "thread.json")); err == nil {
			_ = json.Unmarshal(raw, &meta)
		}
		dslug := sanitizeSlug(meta.Slug)
		if dslug == "" {
			dslug = fmt.Sprintf("thread-%03d", i+1)
		}
		dossier, err := os.ReadFile(filepath.Join(genDir, "dossier.md"))
		if err != nil {
			return fmt.Errorf("read dossier: %w", err)
		}
		dst := filepath.Join(stagingDir, dslug+".md")
		if err := os.WriteFile(dst, dossier, 0o644); err != nil {
			return err
		}
		chosen = append(chosen, meta.Title)
		staged++
		overwrite := ""
		if _, err := os.Stat(layout.NotebookFile(dslug)); err == nil {
			overwrite = "  (updates existing dossier; will back up on accept)"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  staged: %-28s %s%s\n", dslug, meta.Title, overwrite)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\n%d dossier(s) staged in %s\nreview with: worldsmith expand review %s\n",
		staged, stagingDir, slug)
	return nil
}

func expandReviewCommand() *cobra.Command {
	var slug, acceptCSV, rejectCSV string
	var acceptAll bool
	cmd := &cobra.Command{
		Use:   "review <slug>",
		Short: "Review staged dossiers and accept or discard each.",
		Long: `review walks staged expansion dossiers interactively (a/e/r/s/q). For
non-interactive / overnight curation, pass --accept-all, or --accept "<slug,slug>"
and/or --reject "<slug,slug>" (anything not named is left staged).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required")
			}
			return runExpandReview(cmd, slug, acceptAll, acceptCSV, rejectCSV)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().BoolVar(&acceptAll, "accept-all", false, "Non-interactive: accept every staged dossier.")
	cmd.Flags().StringVar(&acceptCSV, "accept", "", "Non-interactive: comma-separated slugs to accept (rest left staged unless --reject).")
	cmd.Flags().StringVar(&rejectCSV, "reject", "", "Non-interactive: comma-separated slugs to discard.")
	return cmd
}

func csvSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			m[p] = true
		}
	}
	return m
}

func runExpandReview(cmd *cobra.Command, slug string, acceptAll bool, acceptCSV, rejectCSV string) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	unlock, err := lockWorld(layout)
	if err != nil {
		return err
	}
	defer unlock()
	staged, err := world.ListStaged(layout)
	if err != nil {
		return err
	}
	if len(staged) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "nothing staged for %s — run `worldsmith expand %s` first\n", slug, slug)
		return nil
	}
	backupStamp := time.Now().Format("2006-01-02T15-04-05")
	out := cmd.OutOrStdout()

	// Non-interactive curation (overnight / scripted): act on flags, no prompts.
	if acceptAll || acceptCSV != "" || rejectCSV != "" {
		acc, rej := csvSet(acceptCSV), csvSet(rejectCSV)
		accepted, discarded, skipped := 0, 0, 0
		for _, d := range staged {
			switch {
			case acceptAll || acc[d.Slug]:
				if err := world.AcceptStaged(layout, d, backupStamp); err != nil {
					return fmt.Errorf("accept %s: %w", d.Slug, err)
				}
				accepted++
				fmt.Fprintf(out, "✓ accepted  %s → %s\n", d.Slug, layout.NotebookFile(d.Slug))
			case rej[d.Slug]:
				if err := world.DiscardStaged(d); err != nil {
					return err
				}
				discarded++
				fmt.Fprintf(out, "✗ discarded %s\n", d.Slug)
			default:
				skipped++
				fmt.Fprintf(out, "↷ left staged %s\n", d.Slug)
			}
		}
		fmt.Fprintf(out, "\naccepted %d, discarded %d, left staged %d\n", accepted, discarded, skipped)
		return nil
	}

	// Interactive review: the a/e/r/s/q loop is the shared contentkit.ReviewLoop;
	// we supply the notebook-specific accept/discard/edit actions.
	items := make([]contentkit.ReviewItem, 0, len(staged))
	itemByID := map[string]world.Dossier{}
	for _, d := range staged {
		body, _ := os.ReadFile(d.Path)
		_, isUpdate := os.Stat(layout.NotebookFile(d.Slug))
		items = append(items, contentkit.ReviewItem{
			ID: d.Slug, Title: d.Title, Body: string(body), Stamp: d.Stamp,
			IsUpdate: isUpdate == nil,
		})
		itemByID[d.Slug] = d
	}
	res, err := contentkit.ReviewLoop(os.Stdin, out, items, contentkit.ReviewActions{
		Accept:  func(it contentkit.ReviewItem) error { return world.AcceptStaged(layout, itemByID[it.ID], backupStamp) },
		Discard: func(it contentkit.ReviewItem) error { return world.DiscardStaged(itemByID[it.ID]) },
		Edit:    func(it contentkit.ReviewItem) error { return editFileInEditor(cmd.Context(), itemByID[it.ID].Path) },
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\naccepted %d, discarded %d\n", res.Accepted, res.Discarded)
	return nil
}

func editFileInEditor(ctx context.Context, path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	c := exec.CommandContext(ctx, editor, path)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// sanitizeSlug keeps a model-proposed slug safe as a filename stem.
func sanitizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == ' ' || r == '_':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
