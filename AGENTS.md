# AGENTS.md

Guidance for AI agents (and humans) working in this repo. See `README.md` for the
product story; this file is the engineering quick-reference.

## Repo at a glance

worldsmith is a Go CLI that treats each *world* as a virtual author: a human-authored
bible + a machine-grown private notebook + accumulated canon, turned into audiobook
m4bs and short-form video. It is a **thin orchestrator over
[vibe](https://github.com/gallowaysoftware/vibe)**: it builds vibe/vamp pipelines and
delegates execution (LLM, TTS, ComfyUI) to the vibe daemon. It does not run models
itself.

- `cmd/worldsmith/` — the cobra CLI. `main.go` registers the root command and most
  subcommands; per-command files (`ask_cmd.go`, `codex_cmd.go`, `score_cmd.go`,
  `autopilot_cmd.go`, `expand_cmd.go`, `timeline.go`, `timeline_generate.go`,
  `contentmill.go` for `worldgen`/`scene`, `bench.go`) hold the rest. Each command
  builds a pipeline and runs it via `vamp.BuildRoot(...)` + `root.SetArgs([...])`.
- `internal/pipeline/` — pipeline builders (`BuildStory`, `BuildBrief`, `BuildArc`,
  `BuildExpand`, `BuildAsk`, `BuildCodex`, `BuildScene`, `BuildWorldGen`, …). Each
  declares its profile/service requirements (`RequireProfile("long_form")`,
  `RequireService("kokoro-tts"/"comfyui", …)`) and prose-stage prompts.
  `prompts/*.md` are the stage prompt templates; `workflows/` holds ComfyUI graphs.
- `internal/world/` — the on-disk model: layout/state, canon, briefs, arcs, series,
  notebook, timeline, scorecards, metrics. This is where most unit tests live.

State for a world lives at `~/.local/state/worldsmith/<slug>/` (see README's "World
layout"). The LLM never edits the bible (`world.md` / `characters.json`); every
machine addition is *staged* for human accept/edit/reject and accepts are backed up.

## Inner loop

```bash
go build ./...        # compile everything
go test ./...         # run the unit tests (fast; mostly internal/world)
go vet ./...          # static checks
gofmt -l .            # list unformatted files (should be empty)
```

There are no model-dependent tests — `go test ./...` runs without vibe/GPU.
Running the actual CLI (`worldsmith story …` etc.) requires a running vibe daemon
with the `long_form` / `tts_kokoro` / `comfyui` profiles+services (see README
"Prerequisites"); `worldsmith doctor` checks they're up.

## Command surface

Generation/authoring: `init`, `worldgen`, `brief`, `arc`, `novel`, `series plan`,
`series write`, `story`, `scene`, `expand`, `expand review`.
Query/measure/maintenance: `ask`, `codex`, `score`, `autopilot`, `bench`,
`timeline {list,show,add,review,generate}`, `list`, `activate`, `doctor`.
Most take the world slug positionally or via `--slug`. Run `worldsmith <cmd> --help`
for the authoritative flag list — it is generated from the cobra definitions.

## Conventions

- **Slugs** are lowercase letters / digits / hyphens (`isSlug`); they double as the
  on-disk world dir name, so keep them filesystem-safe.
- **Human-owns-the-bible:** never write code that edits `world.md` / `characters.json`
  on the user's behalf. Machine output is staged under `.expand/`, `.gen*/`, etc., and
  merged only on explicit accept; overwrites back up the prior version first.
- **Pipelines, not ad-hoc calls:** add new generation by writing a `Build*` in
  `internal/pipeline/` that declares its `RequireProfile`/`RequireService` and runs
  through vamp — don't call models directly from `cmd/`.
- **Capability hints:** text stages use the `long_form` capability/profile and hint a
  ≥27B, ≥128k-context model. Keep new stages consistent unless there's a reason.
- Idempotent commands (`init`, `activate`, `novel` resume, finished-installment skip)
  are the norm — preserve that when extending them.
- Standard Go style: `gofmt`, table-driven tests next to the package under test, errors
  wrapped with `%w`. Keep new tests in `internal/...` so they stay model-free.

## Commit conventions

Commit messages end with the standard co-author trailer used in this project. Branch
for changes; do not push without being asked.
