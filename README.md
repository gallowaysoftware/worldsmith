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

---

## Install & bring up services

```bash
go install github.com/gallowaysoftware/worldsmith/cmd/worldsmith@latest

worldsmith activate     # start the LLM (long_form) + Kokoro TTS + ComfyUI
worldsmith doctor       # read-only: what's up, what's missing
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
worldsmith story my-world --candidates 4  # generate 4 outlines, auto-pick the strongest
```

### Novel → multi-hour audiobook
```bash
worldsmith arc my-world --premise "the long war's first year" --chapters 25
#   → drafts arc.json (chapter spine) — edit it
worldsmith novel my-world --target-chapters 25   # per-chapter, stitched into book.m4b
```

### Short-form video (vertical, captioned)
```bash
worldsmith scene my-world --shots 7       # phase 1 writes the shot list (LLM)…
#   …phase 2 renders stills → image-to-video → voiceover → final.mp4
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
├── arc.json            Optional. Novel chapter spine.
├── notebook/           The author's PRIVATE notes — grown by `expand`, accepted by you.
│   ├── <thread>.md
│   └── .backups/<ts>/  Prior versions of any overwritten dossier (reversible).
├── .expand/<ts>/       Staged expansion PROPOSALS awaiting `expand review`.
├── briefs/             YOU write/edit. One brief per installment.
│   └── NNN.md
├── timeline/           Optional. Historical events + fog-of-war visibility.
└── installments/       Auto-written outputs.
    └── NNN/
        ├── story.md  summary.md  canon_delta.md  cover.png  episode.m4b
```

---

## Commands at a glance

| Command | What it does |
|---------|--------------|
| `init <slug>` | Scaffold a new world to hand-author. |
| `worldgen --theme ...` | LLM-draft a starting world from a theme. |
| `expand <slug> [--seed ...] [--count N]` | Grow the private notebook (proposals). |
| `expand review <slug>` | Accept / edit / reject staged dossiers. |
| `brief <slug> [--steer ...]` | Draft the next installment's direction (you edit it). |
| `story <slug> [--candidates N] [--publish-to ...]` | Short story → audiobook. |
| `arc <slug>` / `novel <slug>` | Novel chapter spine → multi-hour audiobook. |
| `scene <slug> [--shots N]` | Short-form vertical video. |
| `timeline ...` | Manage + review the historical timeline (fog-of-war). |
| `list` | All worlds + installment counts. |
| `activate` / `doctor` | Bring services up / health check. |
| `bench` | A/B candidate model profiles on a fixed installment. |
