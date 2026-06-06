package main

import (
	"context"
	"os"
	"os/exec"
)

// newEditorCmd builds an exec.Cmd that launches $editor against
// path, wired to the parent's stdio so the user sees their normal
// terminal. Split into its own file (and out of timeline.go) so
// the editor-launch pattern is reusable from future subcommands
// (e.g. `worldsmith brief edit`). The context lets a Ctrl-C
// propagate to the editor instead of orphaning it.
func newEditorCmd(ctx context.Context, editor, path string) *exec.Cmd {
	c := exec.CommandContext(ctx, editor, path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}
