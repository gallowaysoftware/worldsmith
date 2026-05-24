# worldsmith

Human worldbuilding → LLM-extrapolated fiction → audiobook. You write the world bible + a per-installment brief; worldsmith writes the prose, narrates it, and stitches it into an m4b that drops into your audiobook library.

Built on top of [vibe](https://github.com/gallowaysoftware/vibe). Shares the autoregressive-canon pattern from [fake-crime](https://github.com/gallowaysoftware/fake-crime): an immutable world bible plus a growing canon document that every new installment reads before drafting.

## What's the difference from fake-crime

- **You supply the world.** fake-crime generates a bible from a one-line concept; worldsmith expects you to hand-write `world.md` + `characters.json` and have opinions about them. The LLM does not invent your setting.
- **Briefs between installments.** Instead of a fixed 10-beat arc, each installment is driven by a `brief.md` you write ("this one is about the harbor blockade, introduce the cartographer"). The LLM extrapolates within the brief.
- **Two scopes**: `story` for a short-form standalone piece (~5-10k words / ~30-45 min m4b), `novel` for a multi-chapter long-arc audiobook (~50-80k words / ~6-8 hour m4b). Both share the same world bible + canon.

## Quickstart

```bash
go install github.com/gallowaysoftware/worldsmith/cmd/worldsmith@latest

# Start a new world. Drops a stub layout into
# ~/.local/state/worldsmith/<slug>/ that you fill in.
worldsmith init driftwood-vale

# Edit ~/.local/state/worldsmith/driftwood-vale/world.md and
# characters.json with your setting + cast. Optionally drop a
# brief.md describing what the first installment is about.

# Bring up everything the pipeline needs (long_form LLM + Kokoro
# TTS + ComfyUI for covers).
worldsmith activate

# Generate the first short story.
worldsmith story driftwood-vale

# Or a novel-length arc (multi-chapter).
worldsmith novel driftwood-vale --target-chapters 25
```

Each finished installment lands at `~/.local/state/worldsmith/<slug>/installments/<NNN>/episode.m4b` with prose summary, canon delta, and per-chapter scripts alongside for audit / review.

## World layout

```
~/.local/state/worldsmith/<slug>/
├── world.md              You write. Setting, magic, factions, history, tone.
├── characters.json       You write. Named cast with arcs / traits / voice notes.
├── arc.json              Optional. High-level beats if you want a fixed arc;
│                         otherwise per-installment briefs drive.
├── canon.md              Auto-grown. Concatenated canon_delta from each installment.
├── briefs/               You write. One brief.md per installment numbered to match.
│   ├── 001.md
│   ├── 002.md
│   └── ...
└── installments/         Auto-written outputs.
    ├── 001/
    │   ├── transcript.md
    │   ├── canon_delta.md
    │   ├── summary.md
    │   ├── cover.png
    │   └── episode.m4b
    └── ...
```
