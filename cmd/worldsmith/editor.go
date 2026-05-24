package main

import (
	"os"
	"os/exec"
)

// newEditorCmd builds an exec.Cmd that launches $editor against
// path, wired to the parent's stdio so the user sees their normal
// terminal. Split into its own file (and out of timeline.go) so
// the editor-launch pattern is reusable from future subcommands
// (e.g. `worldsmith brief edit`).
func newEditorCmd(editor, path string) *exec.Cmd {
	c := exec.Command(editor, path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c
}
