package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// resolveEditor picks the user's editor: $VISUAL, then $EDITOR, then vi.
func resolveEditor() string {
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

// newEditorCmd builds an exec.Cmd that launches $editor against
// path, wired to the parent's stdio so the user sees their normal
// terminal. Split into its own file (and out of timeline.go) so
// the editor-launch pattern is reusable from future subcommands
// (e.g. `worldsmith brief edit`). The context lets a Ctrl-C
// propagate to the editor instead of orphaning it.
func newEditorCmd(ctx context.Context, editor, path string) *exec.Cmd {
	// $EDITOR/$VISUAL may carry arguments (e.g. "code --wait", "emacsclient
	// -nw"). Running the whole string as the binary name would fail; split
	// into the binary plus its leading args and append the path last.
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		fields = []string{"vi"}
	}
	args := append(fields[1:], path)
	c := exec.CommandContext(ctx, fields[0], args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}

// editFileInEditor opens path in the user's editor and waits for it to exit. The single
// launcher used by every interactive-edit subcommand.
func editFileInEditor(ctx context.Context, path string) error {
	return newEditorCmd(ctx, resolveEditor(), path).Run()
}
