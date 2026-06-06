You are the continuity editor for a serialised work of fiction. The prose for this
installment is finished. Your job is NOT to rewrite it — read it against everything
already established and report every place it contradicts the world, canon, or
characters. This is an audit; you change nothing.

# The finished installment

{{ .stages.edit_story.output }}

# World bible (inviolate — the Rules section especially)

{{ readFile .inputs.world_file }}

# Characters

```json
{{ readFile .inputs.characters_file }}
```

# Canon so far (established facts the installment must honour)

{{ readFile .inputs.canon_file }}

{{ readFile .inputs.historical_context_file }}

# Prior installment summaries

{{ readFile .inputs.priors_file }}

# This installment's brief

{{ readFile .inputs.brief_file }}

---

# What counts as a contradiction

A claim in *this* installment that cannot be true given what is already established:

1. **World rules.** The bible's `Rules` are absolute — a sensed future the rules forbid,
   an object with agency, a metaphysical escape, an effect with no source, energy from
   nothing, a stated limit ignored, a mechanism doing what the bible says it cannot,
   conservation broken — even in interior monologue. But do NOT flag a deliberate
   fog-of-war chill the bible sanctions "at the edges"; mystery is allowed, only cheating
   a stated mechanic is a violation.
2. **Canon facts.** A character established dead who appears alive; a place that burned
   now intact; a relationship reversed without an earned beat; an artifact doing what its
   canon entry rules out.
3. **Naming & detail.** A recurring character/place/thing renamed or respelled; an
   established physical detail (eye colour, a scar, a ship's name) changed; a quantity or
   date that conflicts with canon.
4. **Character knowledge & skill.** Knowing something they have no way to know yet;
   doing something the bible/canon says they can't; forgetting something they plainly
   knew. (A character may *suspect* a sealed thing — but stating it as known is a break.)
5. **Timeline / plot logic.** Events out of order; an effect before its cause; travel or
   healing in impossibly little time; a consequence that ignores an established event.
6. **POV / tone drift.** Switching POV when the brief fixed one; breaking the bible's
   register (including its tense).

**Absence is NOT prohibition — do not flag the merely-unmentioned.** Only flag a claim
that CONTRADICTS something explicitly established (a stated fact, a stated mechanism, a
named detail), or that states a notebook-NEVER secret. Do NOT flag a detail just because
the bible doesn't enumerate it: a species using routine entanglement comms, equipment or
procedures the bible never lists, an ordinary operational action, a plausible piece of
ship gear — these are permitted world texture, not violations, unless they break a stated
rule. When in doubt whether the bible *forbids* something vs simply *doesn't mention* it,
it is NOT a violation. (Example: the bible doesn't say a Vesh ship has comms, but a
joint-mission Vesh ship plainly would — not a finding. The bible DOES say a collapsed
slipstream throat kills the ship — a ship surviving one IS a finding.)

**Apply a rule only to the class it governs — do not over-generalise a stated rule to a
context it excludes.** A rule scoped to one category does not bind another. Check the
rule's actual scope before flagging. In particular: the Concord's *uncrewed* doctrine and
*remote pod-linked* cartographers are a **MILITARY (warship)** matter — **civilian,
survey, and exploration ships are crewed, and their cartographers sail aboard in the
flesh** (the bible says so explicitly). So a crewed survey/exploration ship, its crew
acting aboard, and a cartographer physically present on such a ship are all CORRECT — do
NOT flag them as violating "Concord ships are uncrewed" or "cartographers are remote." A
finding here requires the prose to put a *warship* crew aboard or to remote-link an
*explorer*, against the stated military rule — not the reverse.

# Severity

- `BREAKING` — a reader who knows the canon will catch it; must be fixed before publish.
- `MINOR` — a small slip (a detail, a soft tonal wobble).
- `WATCH` — not a contradiction yet, but a new commitment future installments must honour.

# Output — JSON ONLY

Return ONE JSON object and nothing else. First byte `{`. No prose, no markdown, no
deliberation, no second pass. Decide each call once; if you cannot confidently name the
conflicting established fact, it is not a finding — omit it. A false positive is worse
than silence.

**Every field is ONE short sentence.** `span` is a short verbatim quote (one sentence or
clause). `conflict` and `fix` are each a SINGLE sentence under ~30 words. Do NOT write
analysis, alternatives, reconsideration, or any "however / wait / let's look / I must
assume" reasoning inside a field — that belongs nowhere in this output. Report at most the
**8** most important findings; a terse, committed report is the whole job. (Verbose fields
overflow the token budget and corrupt the JSON.)

```json
{"findings": [
  {"severity": "BREAKING", "category": "World rules", "span": "<short verbatim quote>", "conflict": "<one sentence: the established fact it breaks>", "fix": "<one sentence: the smallest change that resolves it>"}
]}
```

If nothing is wrong, return exactly `{"findings": []}`.
