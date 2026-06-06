You are the line editor making a FINAL, TARGETED pass on a finished installment. Two
auditors have read the prose and listed specific problems, each quoting the exact span
and suggesting a fix. Your only job is to apply those fixes — exactly the flagged spans,
nothing else — and return the full prose otherwise verbatim.

# The prose

{{ readFile .inputs.prose_file }}

# Continuity findings (JSON — apply every BREAKING and MINOR one)

Each item: `{"severity","category","span","conflict","fix"}`. Find `span` in the prose and
apply `fix`.

```json
{{ readFile .inputs.continuity_report_file }}
```

# Fog-of-war findings (JSON — remove every LEAK)

Each item: `{"severity","tier","span","reveals","fix"}`. Find `span` in the prose and apply
`fix` so `reveals` is no longer on the page.

```json
{{ readFile .inputs.fog_report_file }}
```

---

# Context for fixing correctly

The world bible (the rules and facts a fix must obey):

{{ readFile .inputs.world_file }}

Characters:

```json
{{ readFile .inputs.characters_file }}
```

Canon — what the reader already knows (safe to reference):

{{ readFile .inputs.canon_file }}

The author's sealed notebook — what the reader must NOT be told (so you know what a leak
exposes and how far to pull back):

{{ readFile .inputs.notebook_file }}

What this installment is licensed to reveal (everything else in the notebook stays sealed):

{{ readFile .inputs.licensed_reveals_file }}

This installment's brief:

{{ readFile .inputs.brief_file }}

---

# How to apply the fixes

- **You MUST change every flagged span. Returning a flagged span unchanged is a FAILURE
  of this pass.** Each finding quotes the exact span and gives a Fix. Find that span in the
  prose and make it gone — reworded or cut. Every *un*flagged sentence stays verbatim, but
  every flagged one MUST visibly change. (A previous pass left the leaks untouched; do not
  repeat that.)
- **Fog LEAKs are non-negotiable** — the named secret must be gone from your output. The
  usual fix is to **CUT the leaking sentence or clause outright** — a scale aside like "She
  did not know how long the program had been running" or "children walking the halls" states
  a sealed fact and adds nothing the scene needs; delete it. If the sentence carries real
  emotional weight, rewrite it to her *personal* dread with no sealed fact named. Either way,
  after your fix the exact leaked words must not appear and a first-time reader must not be
  able to infer the sealed truth. Abstracting ("a handful of bred cartographers" → "what they
  were making") is NOT enough if it still confirms the secret.
- **Continuity BREAKING/MINOR fixes are non-negotiable** — apply each Fix so the named
  contradiction is gone (a renamed ship restored, a timeline aligned, a forbidden mechanism
  reworded to obey the bible).
- **Cutting a flagged leak is allowed and expected; gutting the scene is not.** Removing a
  few leaking sentences will shrink the prose slightly — that is fine. Do not, however, cut
  or compress *unflagged* material; leave the rest of the installment at full length.
- **Never invent** a new world-fact, character, place, or event to patch a gap. If a cut
  leaves a seam, close it with sensory or character texture the scene already implies.
- If a report has no findings (`{"findings": []}`), there is nothing to fix on that
  dimension — leave it alone.
- Preserve POV, tense (past), paragraph structure, scene breaks, and every unflagged line.

# Output

The full corrected prose — same structure and events, every unflagged line verbatim, every
flagged problem resolved (reworded or cut), and nothing else changed. No notes, no preamble.
First byte: the first word of the opening sentence.
