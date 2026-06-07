You are a continuity AND fog-of-war editor. Below are specific spans from a finished
story installment, each flagged either as a continuity contradiction or as a fog-of-war
leak (sealed material stated on the page). For EACH span, produce its replacement text —
the exact words that should stand in its place. You are given only the spans, not the
whole story; you do not need it. Keep replacements short and local: a span is usually one
sentence, so its replacement is usually one sentence.

# Continuity findings (JSON) — rewrite each so the contradiction is gone

Each: `{"severity","category","span","conflict","fix"}`. Reword the span so the named
contradiction is gone, obeying the world rules and brief below. A BREAKING factual error
(a renamed ship, a mechanism that disobeys the bible) must be reworded so the wrong claim
is no longer made.

```json
{{ readFile .inputs.continuity_report_file }}
```

# Fog-of-war leaks (JSON) — rewrite each to keep the secret as SUBTEXT

Each: `{"severity","tier","span","reveals","fix"}`. A LEAK is a span that STATES, confirms,
or plainly implies sealed material the reader must not learn yet. **Do not just delete it —
rework it.** Keep the scene's beat, motion, and feeling, but render the sealed thing only
as subtext: what a character notices, avoids, fears, or won't name — never the thing
itself. Strip the specific that gives it away (a count, a name, a label, a mechanism, a
"this means X" generalisation) and leave the grounded, oblique moment. The reader should
feel the weight without being told the secret.

Example shape: "He thought of the nineteen others Doran had broken before her." → "He did
not let himself count the ones who had come before her." (the dread stays; the number, and
what it implies, is gone.)

```json
{{ readFile .inputs.fog_report_file }}
```

# Context (obey these when rewording)

World rules (the mechanism a fix must obey):

{{ readFile .inputs.world_file }}

The sealed notebook (NEVER introduce anything from here that the brief does not license —
this is the material a fog rework must keep hidden):

{{ readFile .inputs.notebook_file }}

Licensed to reveal this installment (sealed material the brief DOES permit on the page —
do not strip these out as leaks):

{{ readFile .inputs.licensed_reveals_file }}

This installment's brief:

{{ readFile .inputs.brief_file }}

---

# Output — JSON ONLY

For every finding above (continuity AND fog), output one replacement object. First byte
`{`. No prose, no commentary.

- `span` — copy the finding's span VERBATIM (so it can be located in the prose).
- `replacement` — the words to put in its place. Match the surrounding register and past
  tense. Do NOT add a sealed fact, a new character, or a new event. For a fog leak, the
  replacement MUST NOT state or imply the sealed thing. If a wrong/leaking claim simply
  should not be there at all, you may return `""` to cut it — but prefer a subtext rework
  that keeps the scene whole.

```json
{"replacements": [
  {"span": "<verbatim span from a finding>", "replacement": "<the corrected/reworked text, or \"\" to cut>"}
]}
```
