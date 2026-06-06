package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gallowaysoftware/vibe/vamp"

	"github.com/gallowaysoftware/worldsmith/internal/pipeline"
	"github.com/gallowaysoftware/worldsmith/internal/world"
)

// askCommand answers a question about a world from the author's full knowledge —
// bible + canon + the private notebook (no fog of war: you're asking yourself).
// Makes the universe queryable, not just generatable.
func askCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ask <slug> <question...>",
		Short: "Ask the author a question about the world (draws on bible + canon + private notebook).",
		Long: `ask consults the world's full knowledge — the published bible, the canon
established so far, and the private notebook (secrets, where threads are going) —
and answers in the author's voice. Unlike generation, it reveals freely: you are
the author asking yourself. Good for "what does Crane actually want?", "is X
consistent with Y?", "where could this thread go?".`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAsk(cmd, args[0], strings.Join(args[1:], " "))
		},
	}
	return cmd
}

func runAsk(cmd *cobra.Command, slug, question string) error {
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
	genDir := filepath.Join(layout.Root, ".ask", time.Now().Format("2006-01-02T15-04-05"))
	if err := os.MkdirAll(genDir, 0o755); err != nil {
		return err
	}
	notebookPath, err := world.WriteAssembledNotebook(layout, genDir)
	if err != nil {
		return fmt.Errorf("assemble notebook: %w", err)
	}

	cfg := pipeline.AskConfig{
		WorldFile:      layout.WorldFile(),
		CharactersFile: layout.CharactersFile(),
		CanonFile:      canonPath,
		NotebookFile:   notebookPath,
		Question:       question,
	}
	fmt.Fprintf(cmd.OutOrStdout(), "world: %s\nQ: %s\n\n", slug, question)

	root, err := vamp.BuildRoot(func() (*vamp.Pipeline, error) {
		return pipeline.BuildAsk(cfg)
	})
	if err != nil {
		return err
	}
	root.SetArgs([]string{"run", "--run-dir", genDir, "--no-cache"})
	if err := root.ExecuteContext(cmd.Context()); err != nil {
		return fmt.Errorf("ask: %w", err)
	}
	answer, err := os.ReadFile(filepath.Join(genDir, "answer.md"))
	if err != nil {
		return fmt.Errorf("read answer: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s\n", strings.TrimSpace(string(answer)))
	return nil
}
