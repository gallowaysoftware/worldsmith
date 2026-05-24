// Package pipeline holds the worldsmith vamp pipeline + its embedded
// prompts and ComfyUI workflows. The CLI binary (cmd/worldsmith)
// drives it; this package owns the DAG.
package pipeline

import (
	"embed"
	"io/fs"
)

//go:embed prompts/*.md workflows/*.json
var assets embed.FS

// PromptsFS narrows the embed to prompts/ so PromptFS calls
// reference each template by its bare filename.
var PromptsFS fs.FS = mustSub(assets, "prompts")

// WorkflowsFS exposes ComfyUI workflow JSON files (sdxl_turbo.json
// for cover-art generation) to the ComfyUI stage via WorkflowFS.
var WorkflowsFS fs.FS = mustSub(assets, "workflows")

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
