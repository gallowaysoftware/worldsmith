# worldsmith

**Each world is a virtual author.** You set the stage — the setting, the cast, the
rules — and worldsmith becomes the author who has lived inside that universe for
years: all their notes, their secrets, the threads they haven't pulled yet, the
places the story could still go. Generating a story, a novel, or a short is that
author sitting down with their notebook and writing the next piece of their life's
work.

Built on [vibe](https://github.com/gallowaysoftware/vibe). Human worldbuilding →
LLM extrapolation → audiobook m4bs (and short-form video).

---

## The mental model: three layers

A world is **not** one document. It's three, and keeping them separate is what
makes the author feel real:

| Layer | File(s) | Who writes it | What it is |
|-------|---------|---------------|------------|
| **The bible** | `world.md`, `characters.json` | **You** (the LLM never edits these) | The published setting: history, factions, tone, the absolute rules. The stage you set. |
| **Canon** | `canon.md` | Auto-grown from installments | The facts you've *shown readers* so far. Grows append-only as you publish. |
| **The notebook** | `notebook/*.md` | Grown by `expand`, **you approve** | The author's **private** knowledge: secrets not yet revealed, where threads are going, deep character interiority, faction true-agendas. What the author knows; readers don't. |

The notebook is the new idea: it's the author's brain. When the author writes the
next piece, they draw on everything in the notebook for depth, subtext, and
foreshadowing — while *fog-of-war* keeps the secrets secret until you choose to
reveal them.

> **Safety promise:** the LLM never edits your bible, and never folds anything into
> a world without your say-so. Every machine-generated addition is *staged* for you
> to **accept, edit, or discard**. Accepts are reversible (the prior version is
> backed up).

### Glossary

- **Bible** — the published setting *you* author and own: `world.md` (history,
  factions, tone, the rules) + `characters.json` (the cast). The LLM reads it but
  never edits it.
- **Rules** — the load-bearing "rules for the writer" section inside `world.md`:
  your hard constraints ("no magic," "honor the calendar"). Every later stage,
  including the expansion critic, enforces them.
- **Canon** (`canon.md`) — the facts you've actually *shown readers* so far.
  Grows append-only as you publish installments; what generation may freely
  reference.
- **Notebook** (`notebook/*.md`) — the author's-notebook layer: the author's
  **private** knowledge (unrevealed secrets, where threads are going, deep
  interiority). Grown by `expand`, accepted by you; drawn on for depth/subtext but
  kept hidden from readers.
- **Dossier** — one notebook entry: a single developed thread covering what's
  established, the private truth, where it's going, interiority, connections, and
  reveal control. `expand` produces dossiers as staged proposals.
- **Brief** (`briefs/NNN.md`) — the per-installment direction document you write
  and edit; the `story`/`novel` pipeline generates *from* it.
- **Fog-of-war** — the reveal-control discipline that keeps notebook secrets and
  not-yet-visible timeline events out of generated text until you choose to surface
  them; timeline events carry visibility tiers that decide which stories see them.

---

## Prerequisites

worldsmith is a thin orchestrator on top of [vibe](https://github.com/gallowaysoftware/vibe):
it builds vibe/vamp pipelines and asks vibe to run the models. It does **not** ship
or manage models itself. Before anything works you need:

- **vibe installed and its daemon running.** Every generation command (`story`,
  `novel`, `series`, `brief`, `arc`, `expand`, `ask`, `codex`, `scene`,
  `worldgen`, `timeline generate`, `score`, `bench`) shells out to vamp, which
  talks to the vibe daemon. If vibe isn't up, these commands fail.
- **The vibe profiles / services worldsmith requires** (these are the names vamp
  resolves — they must exist in your vibe config):
  - **`long_form`** — the LLM profile every text stage uses. The pipelines hint a
    ≥27B, ≥128k-context model (e.g. `qwen3.6-27b-mtp-q6_k`); the OpenAI-compatible
    endpoint is expected at `http://127.0.0.1:9001`.
  - **`tts_kokoro`** — Kokoro-FastAPI narration TTS, expected at
    `http://127.0.0.1:8880` (needed by `story`, `novel`, `series`, `scene`).
  - **`comfyui`** — ComfyUI for cover-art (SDXL) and short-video generation
    (Qwen-Image stills + Wan2.2 image-to-video), expected at
    `http://127.0.0.1:8188` (needed by `story`/`novel`/`series` covers and by
    `scene`).
- **`ffmpeg`** on `PATH` for audio mixing and stitching multi-chapter `.m4b`s
  (without it the per-chapter/per-installment files still stand on their own).
- A GPU with headroom: a `story` run wants ~30GB during the LLM stages, ~6GB
  during TTS, ~4GB during SDXL (the stages run sequentially, not all at once).

`worldsmith activate` brings the above up by delegating to vamp's `activate`
(the same plumbing vibe uses per-pipeline); it's idempotent, leaving
already-running services alone. `worldsmith doctor` is the read-only check: it
probes each required profile/service and exits non-zero if anything is missing,
so it doubles as a CI gate.

## Install & bring up services

```bash
go install github.com/gallowaysoftware/worldsmith/cmd/worldsmith@latest

worldsmith activate     # start the LLM (long_form) + Kokoro TTS + ComfyUI via vamp
worldsmith doctor       # read-only: which required profiles/services are up
```

Everything lives under `~/.local/state/worldsmith/<slug>/`.

---

## 1. Create a world

Two ways to set the stage:

```bash
# A) Hand-author it (the intended path — you have opinions about your world).
worldsmith init my-world
#   → edit ~/.local/state/worldsmith/my-world/world.md  (setting, history, factions,
#     tone, and an absolute "rules for the writer" section the LLM must obey)
#   → edit characters.json  (named cast: role, look, voice, arc notes)

# B) Let the LLM draft a starting world from a theme, then take it over.
worldsmith worldgen --theme "deep-sea archive city run by retired spies" --slug my-world
#   → then edit world.md / characters.json to make it yours.
```

The `world.md` "rules" section is load-bearing: it's what every later stage
(including the expansion critic) enforces. State your hard constraints there ("no
magic," "honor the calendar," "no aliens beyond the Vesh," etc.).

---

## 2. Expand the world (grow the author's notebook)

This is how the author deepens their universe — pulling threads into private
dossiers. It **never touches `world.md`**; it writes *proposals* you review.

```bash
# Auto: the author finds the richest unpulled threads and develops them.
worldsmith expand my-world --count 3

# Seeded: develop a specific idea you hand it.
worldsmith expand my-world --seed "what the head of the secret police actually wants"
```

Each run writes proposed dossiers to `~/.local/state/worldsmith/my-world/.expand/<timestamp>/`.
A dossier covers: what's *established*, the *private truth* (the secrets), *where
it's going*, *interiority & texture*, *connections* to other threads, and
*reveal control* (what readers know vs. what stays hidden).

Under the hood each thread goes through a writers' room (four lenses: historian,
psychologist, plot-architect, contrarian) → a **canon-consistency critic** that
catches anything contradicting your bible or breaking your stated rules → a revise
pass that fixes every note. Generic, rule-breaking, or off-canon material gets
caught before it reaches you.

### Review and accept

```bash
worldsmith expand review my-world
```

Walks you through each staged dossier:

- **`a` accept** → merges it into `notebook/<slug>.md` (backs up any existing
  version under `notebook/.backups/` first — reversible).
- **`e` edit then accept** → opens it in `$EDITOR` so you can rewrite anything,
  then accepts your edited version.
- **`r` reject** → discards the proposal; the notebook is untouched.
- **`s` skip** → leaves it staged to decide later.
- **`q` quit** → stops; everything not yet decided stays staged.

---

## 3. Modify anything

Everything is plain files you own — edit them directly whenever you like:

- **The bible:** edit `world.md` / `characters.json` in any editor. Yours alone.
- **A notebook dossier:** edit `notebook/<slug>.md` directly, or re-run
  `expand --seed` to develop it further and review the result. Delete a dossier you
  no longer want (a backup remains under `notebook/.backups/` if it was ever
  overwritten).
- **A brief** (per-installment direction): see below — you always edit briefs before
  generating.
- **The timeline:** `worldsmith timeline review my-world` accepts/edits/rejects
  proposed historical events the same `a/e/r/s/q` way; fog-of-war visibility tiers
  control which events surface to which stories.

Nothing you generate is final until you accept it, and nothing the LLM proposes
overwrites your work without a backup.

---

## 4. Generate content

All generation reads the bible + canon (and draws on the notebook for private
depth). You steer each piece with a **brief** you write and edit first.

### Short story → ~30–45 min audiobook
```bash
worldsmith brief my-world --steer "the cartographer is captured; introduce the Vault"
#   → drafts briefs/NNN.md — EDIT IT, it's your direction document
worldsmith story my-world                 # prose → narration → episode.m4b
worldsmith story my-world --best-of 4     # write the prose 4 times, ship the cleanest
```

`story` flags: `--slug` (alternative to the positional arg), `--installment N`
((re)generate a specific number instead of the next pending one — useful for
iterating on a draft), `--narrator <voice>` (Kokoro voice id, default
`am_fenrir`), `--best-of N` (write the prose N times and ship the lowest-badness
convergence; narration runs once, on the winner; default `1` = single pass), and
`--publish-to <dir>` (below). `novel`, `series write`, and `scene` accept the
same `--narrator` / `--publish-to`.

### Novel → multi-hour audiobook
```bash
worldsmith arc my-world --premise "the long war's first year" --chapters 25
#   → drafts arc.json (chapter spine) — edit it
worldsmith novel my-world --target-chapters 25   # per-chapter, stitched into book.m4b
```

> Note the flag names differ on purpose: `arc` takes `--chapters N` (the target
> length of the spine it drafts); `novel` takes `--target-chapters N` (a cap on
> how many of arc.json's chapters to render this run, `0` = all). `novel` also
> writes an `arc.json` stub and stops if one doesn't exist yet.

### Multi-book series → one chaptered .m4b per book
```bash
$EDITOR ~/.local/state/worldsmith/my-world/series.json   # YOU author: arc + books
worldsmith series plan my-world           # drafts arc.json (per-book chapter beats) — edit it
worldsmith series write my-world          # generates chapters → a chaptered .m4b per book
```
(`series plan` writes a `series.json` stub for you to fill in if none exists.)

### Short-form video (vertical, captioned)
```bash
worldsmith scene my-world --shots 7       # phase 1 writes the shot list (LLM)…
#   …phase 2 renders stills → image-to-video → voiceover → final.mp4
```

### Query, document, and measure
```bash
worldsmith ask my-world "what does the head of the secret police actually want?"
#   answers from bible + canon + your private notebook (no fog — you're asking yourself)
worldsmith codex my-world                 # compile a spoiler-safe reader's codex → codex.md
worldsmith score my-world                 # per-installment quality scorecards + trend
```

### Publish to a library
```bash
worldsmith story my-world --publish-to /mnt/media/llm-podcasts/MyWorld/
#   filename from the brief's first H1, zero-padded for chronological order.
```

---

## A full session, end to end

```bash
worldsmith init concord                       # 1. set the stage
$EDITOR ~/.local/state/worldsmith/concord/world.md
worldsmith activate                           #    bring services up

worldsmith expand concord --count 3           # 2. author deepens the universe
worldsmith expand review concord              #    you accept/edit/reject the dossiers

worldsmith brief concord --steer "..."        # 3. direct the next installment
$EDITOR ~/.local/state/worldsmith/concord/briefs/001.md
worldsmith story concord --publish-to ~/media/Concord/   #    write + narrate + publish
```

Each finished installment writes `summary.md` + `canon_delta.md` beside the m4b;
the next `story` reads them automatically so continuity builds without you
re-feeding context. Run `expand` whenever you want the author to think more deeply
before they write.

---

## World layout

```
~/.local/state/worldsmith/<slug>/
├── world.md            YOU write. Setting, factions, tone, the absolute rules. LLM never edits.
├── characters.json     YOU write. The cast.
├── canon.md            Auto-grown (append-only). What readers have been shown.
├── codex.md            Optional. Spoiler-safe companion codex written by `codex`.
├── arc.json            Optional. Novel/series chapter spine (drafted by `arc` or `series plan`).
├── series.json         Optional. YOU write. Multi-book series arc + books (input to `series plan`).
├── notebook/           The author's PRIVATE notes — grown by `expand`, accepted by you.
│   ├── <thread>.md
│   └── .backups/<ts>/  Prior versions of any overwritten dossier (reversible).
├── .expand/<ts>/       Staged expansion PROPOSALS awaiting `expand review`.
├── briefs/             YOU write/edit. One brief per installment.
│   └── NNN.md
├── timeline.json       Optional. Historical events + fog-of-war visibility (or timeline/<era>.json).
├── scenes/             Auto-written short-form video outputs (one dir per `scene`).
└── installments/       Auto-written outputs.
    └── NNN/
        ├── story.md  summary.md  canon_delta.md  cover.png  episode.m4b
```

---

## Commands at a glance

Most commands take the world slug as a positional arg or via `--slug`.

| Command | What it does |
|---------|--------------|
| `init <slug>` | Scaffold a new world to hand-author (world.md + characters.json + briefs/001.md). |
| `worldgen --theme <theme> [--count N] [--slug ...]` | LLM-draft one or more starting worlds from a theme. |
| `expand <slug> [--seed ...] [--count N]` | Develop private notebook dossiers (staged proposals). |
| `expand review <slug> [--accept-all] [--accept/--reject csv]` | Interactively (a/e/r/s/q) or non-interactively accept/reject staged dossiers. |
| `brief <slug> [--steer ...] [--installment N] [--target-words N] [--force]` | Draft the next installment's direction document (you edit it). |
| `story <slug> [--installment N] [--best-of N] [--narrator v] [--publish-to dir]` | Generate the next installment: prose → narration → episode.m4b. |
| `arc <slug> [--premise ...] [--chapters N] [--force]` | Draft a novel's chapter spine (arc.json). |
| `novel <slug> [--target-chapters N] [--best-of N] [--narrator v] [--publish-to dir]` | Render arc.json chapter-by-chapter, stitched into book.m4b. |
| `series plan <slug> [--force]` | Draft per-book chapter beats (arc.json) from a hand-authored series.json. |
| `series write <slug> [--book N] [--chapters N] [--narrator v] [--publish-to dir]` | Generate the series' chapters → one chaptered .m4b per book. |
| `scene <slug> [--shots N] [--format ...] [--narrator v] [--publish-to dir]` | Generate the next short-form vertical (1080×1920, captioned) video. |
| `ask <slug> <question...>` | Answer a question from the author's full knowledge (bible + canon + private notebook; no fog). |
| `codex <slug> [--publish-to dir]` | Compile a spoiler-safe companion codex (codex.md) from bible + canon. |
| `score <slug>` | Show per-installment quality scorecards (prose / continuity / fog) and the trend. |
| `autopilot <slug...> [--max-staged N] [--threads N] [--idle-mib N] [--dry-run]` | Cron loop: when the GPU is idle, `expand` the named worlds and stage proposals for review. |
| `timeline list <slug> [--proposed] [--all]` | List timeline events (canon by default; `--proposed` for LLM drafts). |
| `timeline show <event-id> --slug <slug>` | Pretty-print one event's full record (including the visibility envelope). |
| `timeline add --slug <slug>` | Append a hand-authored event (interactive prompts). |
| `timeline review --slug <slug>` | Walk proposed events; accept / edit / reject each (a/e/r/s/q). |
| `timeline generate --slug <slug>` | Run the five-pass LLM timeline generator; appends proposed events for review. |
| `list` | All worlds + their installment counts. |
| `activate` | Bring up the required vibe profiles/services via vamp (idempotent). |
| `doctor` | Read-only: report which required vibe services are up (non-zero exit if missing). |
| `bench --slug <slug> --installment N --candidates p1,p2[,...]` | A/B candidate `long_form` profiles on a fixed installment; outputs blinded for human scoring. |
