package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/vibe/vamp"

	"github.com/gallowaysoftware/worldsmith/internal/pipeline"
	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// codexCommand compiles a spoiler-safe companion codex (reader's encyclopedia)
// from the world bible + canon, written to <slug>/codex.md.
func codexCommand() *cobra.Command {
	var slug, publishTo string
	cmd := &cobra.Command{
		Use:   "codex <slug>",
		Short: "Compile a spoiler-safe companion codex from the world bible + canon.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				slug = args[0]
			}
			if slug == "" {
				return fmt.Errorf("world slug required")
			}
			return runCodex(cmd, slug, publishTo)
		},
	}
	cmd.Flags().StringVar(&slug, "slug", "", "World slug (alternative to positional arg).")
	cmd.Flags().StringVar(&publishTo, "publish-to", "", "Also copy codex.md into this directory.")
	return cmd
}

func runCodex(cmd *cobra.Command, slug, publishTo string) error {
	layout, err := world.Open(slug)
	if err != nil {
		return err
	}
	if _, err := os.Stat(layout.WorldFile()); err != nil {
		return fmt.Errorf("world.md not found at %s — run `worldsmith init %s` first", layout.WorldFile(), slug)
	}
	canonPath, err := world.EnsureCanonFile(layout)
	if err != nil {
		return fmt.Errorf("ensure canon: %w", err)
	}
	genDir := filepath.Join(layout.Root, ".codex", time.Now().Format("2006-01-02T15-04-05"))
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		return err
	}

	cfg := pipeline.CodexConfig{
		WorldFile:      layout.WorldFile(),
		CharactersFile: layout.CharactersFile(),
		CanonFile:      canonPath,
	}
	fmt.Fprintf(cmd.OutOrStdout(), "world: %s\ncompiling codex...\n\n", slug)

	root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
		return pipeline.BuildCodex(cfg)
	})
	if err != nil {
		return err
	}
	root.SetArgs([]string{"run", "--run-dir", genDir, "--no-cache"})
	if err := root.Execute(); err != nil {
		return fmt.Errorf("codex: %w", err)
	}
	raw, err := os.ReadFile(filepath.Join(genDir, "codex.md"))
	if err != nil {
		return fmt.Errorf("read codex: %w", err)
	}
	dst := filepath.Join(layout.Root, "codex.md")
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "✓ codex written: %s\n", dst)
	if publishTo != "" {
		if err := os.MkdirAll(publishTo, 0o755); err != nil {
			return err
		}
		pdst := filepath.Join(publishTo, slug+"-codex.md")
		if err := copyFileCodex(dst, pdst); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  published: %s\n", pdst)
	}
	return nil
}

func copyFileCodex(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
