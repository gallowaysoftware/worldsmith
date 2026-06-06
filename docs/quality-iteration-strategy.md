# Worldsmith quality-iteration strategy

Written 2026-06-02 after ~6 end-to-end regens of apostolate-concord installment 001.
This captures what we learned, what's proven, and the proposed pipeline evolution to get
from "good with effort" to "reliably high quality."

## What we proved works (keep these)

- **Per-scene authoring fixes length.** Writing each outline scene to its own word budget,
  sequentially (each scene sees the prior prose), reliably hits target length — *as long as
  nothing downstream re-targets length*. A single "write 10k words" pass never honoured length;
  per-scene does.
- **`expand_story` must pass through in the per-scene flow.** It's a length-targeting LLM pass;
  on a per-scene draft it *fights* the budgets (it crushed a 12.5k draft to 6.3k once). Now a
  no-op render when a stitched draft is supplied. Length = sum of scene budgets.
- **`canon.md` as a revealed-to-reader ledger (NOT author memory) is what fixed fog.** Every fog
  leak traced to sealed strategic context (the breeding programme, the two ironies, the "nineteen")
  living in `canon.md`, which the writer read as stateable fact and the fog *precheck* read as
  already-revealed. Moving all sealed material to `notebook/` and keeping canon to genuinely-revealed
  facts took fog from a persistent −30/−35 cap to **clean (100)**. This was the single biggest unlock.
- **Voice-by-host derivation.** Route narration by the `host` slug and derive the Kokoro voice from
  `characters.json`, never from the model's echoed `voice_id` (it typo'd `am_fenrir`→`am_fenfir` and
  400'd TTS). Typos can no longer break a run.
- **Past tense, pinned.** The series register; also relieves the "It is…/She is…" anaphora drumbeat.
- **Route-only showrunner + rejoin.** Narration routing as a compact list (one entry per paragraph),
  text rejoined by index — not the model echoing 12k words of prose into JSON.

## The core problem: variance, and an unreliable fix stage

Two failure modes account for almost all the pain:

1. **Every prompt tweak re-rolls all 7 scenes**, so each run is a fresh dice-throw on
   length/slop/continuity/fog. The score bounced 32 → 65 → 56 → 62 across runs *with the same
   intent*. You cannot converge by re-rolling.
2. **`edit_story` is an omnibus pass** — it applies slop + continuity + fog fixes to the *whole*
   prose in one shot. It under-applies (left fog leaks on the climax), over-cuts (deleted 19% of the
   words instead of rewriting in place), and is non-deterministic. One LLM doing four jobs on 10k
   words does none of them reliably.

Supporting issues: the **precheck that feeds the fix is weaker than the terminal check that scores**
(so flagged issues survive to the score); the **continuity checker rambles** (thinks aloud, second-
guesses, inflates findings); **volatile details get re-hallucinated** each run (ship names);
**anaphora** is a generation tic that prompt rules don't reliably remove.

## The proposed shift: separate GENERATION from POLISH; make polish a verified loop

Stop treating the pipeline as linear `generate → omnibus-edit → ship`. Restructure into phases
where polish iterates on a **frozen** base and runs until the **scoring** checks pass.

### Phase A — Generate, then FREEZE
Per-scene authoring produces the draft. Once a draft is acceptable, freeze it as the working base.
Add a `worldsmith revise <installment>` mode that reuses the frozen scenes/draft and re-runs only
polish + narration. Polish tweaks must never re-roll generation. **This is the variance fix** — you
iterate on a stable base instead of a new dice-throw each time.

### Phase B — Diagnose (disciplined, structured detectors)
Run continuity / fog / prose-slop as *detectors only*, calibrated to emit committed, structured
findings (no rambling, conservative, low temp, thinking-off). They never rewrite.

### Phase C — Targeted fix passes (single-purpose, length-preserving)
Replace the omnibus `edit_story` with a short sequence of **narrow** passes, each acting only on its
own flagged spans, each held to the input's word count:
- **fog-fix** — un-name only the flagged leaks, keep the pressure.
- **continuity-fix** — repair only the flagged contradictions.
- **line-edit** — slop / "not X but Y" / anaphora openers only.

A narrow task with a short, concrete worklist is far more reliable than one pass told to fix
everything. (This is the `templates_over_prompt_rules` lesson applied to editing.)

### Phase D — Verify loop (the reliability keystone)
After the fix passes, **re-run the checks**. If anything is still flagged, fix again — bounded to
N iterations. This closes the precheck/check gap: the loop runs until the *same* check that *scores*
the installment is clean. vamp is a DAG, but the CLI already orchestrates bounded loops (per-scene
authoring is one), so this is a natural CLI-driven loop. **This is the biggest reliability win:** the
shipped artifact provably passes the bar that grades it, instead of hoping a one-shot edit caught
everything.

### Phase E — Narrate + finalize
Unchanged: number-paragraphs → route-only showrunner → enumerate → per-segment TTS → cover → mix.
Consider a **human-approval checkpoint before TTS** (the slow, RAM-heavy, MCE-risky stage): stage the
polished draft + scorecard for a yes/no, then narrate only on approval. Saves re-running TTS on drafts
you'd reject anyway. (Ties into the existing idle-GPU "stage for review" idea.)

## Mechanical fixes (prefer these over prompt rules)

- **Anaphora**: the scorecard already detects over-used sentence openers in Go. Surface that detection
  *in-pipeline* and feed the over-used-opener list to the line-edit pass ("vary these specific
  openers"). Mechanical detect → targeted fix beats "please vary your openers."
- **Pin volatile details**: ship names, dates, counts → pin in the brief / `characters.json` so they
  can't be re-hallucinated or contradicted across runs. (Done: Ila's ship = *Meridian*; her capture is
  explicitly *not* the contact event.)
- **`canon.md` = revealed ledger** (done) and **`reconcile_canon` replaces-on-rerun** (done) so canon
  stays clean automatically instead of needing a manual truncate before each regen.

## Bigger options worth discussing

1. **Best-of-N at the scene level.** Generate 2–3 candidates per scene, judge, keep the best. Trades
   GPU for variance reduction and quality. (Revive the dead `selectBestOutline` machinery, but at scene
   granularity.) Highest quality ceiling; highest GPU cost.
2. **Richer outline / scene specs.** Beat-by-beat scene specs give per-scene generation less room to
   drift on length, leaks, and anaphora. Tighter spec → less variance, before any fixing.
3. **Two-model split.** Strong model for generation; a fast/cheap model for the mechanical detectors and
   narrow fixes. Cheaper, faster polish loops.
4. **Rethink the score.** `overall = min(axes)` is harsh and the continuity axis is noisy. Gate shipping
   on "fog clean AND no breaking continuity" and report axes separately, rather than chasing one number.
   The listen is the goal; the number is a proxy.

## Recommended next step

Build **Phase D (verify loop) + Phase C (targeted fix passes)** first — together they convert the noisy
one-shot edit into a process that provably converges. Then add the `revise` (freeze-and-reuse) mode so
iteration stops re-rolling generation. The mechanical anaphora pass and best-of-N are follow-ons.

All of this needs GPU to build *and validate* (the behaviour only shows in a real run), so it's gated on
card availability.
