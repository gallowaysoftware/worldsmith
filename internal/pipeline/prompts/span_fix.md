You are a continuity editor. Below are specific spans from a finished story installment,
each with a contradiction and a required fix. For EACH span, produce its replacement text —
the exact words that should stand in its place so the contradiction is gone. You are not
given the whole story, only the spans; you do not need it. Keep replacements short and
local: a span is usually one sentence, so its replacement is usually one sentence.

# Continuity findings (JSON) — produce a replacement for every one

Each: `{"severity","category","span","conflict","fix"}`. Reword the span so the named
contradiction is gone, obeying the world rules and brief below. A BREAKING factual error
(a renamed ship, a mechanism that disobeys the bible, a character knowing/stating
something sealed) must be reworded so the wrong claim is no longer made.

```json
{{ readFile .inputs.continuity_report_file }}
```

# Context (obey these when rewording)

World rules (the mechanism a fix must obey):

{{ readFile .inputs.world_file }}

The sealed notebook (do NOT introduce anything from here that the brief does not license):

{{ readFile .inputs.notebook_file }}

Licensed to reveal this installment:

{{ readFile .inputs.licensed_reveals_file }}

This installment's brief:

{{ readFile .inputs.brief_file }}

---

# Output — JSON ONLY

For every continuity finding above, output one replacement object. First byte `{`. No prose,
no commentary.

- `span` — copy the finding's span VERBATIM (so it can be located in the prose).
- `replacement` — the words to put in its place. Match the surrounding register and past
  tense. Do NOT add a sealed fact, a new character, or a new event. If the wrong claim
  simply should not be there, you may return `""` to cut it.

```json
{"replacements": [
  {"span": "<verbatim span from a finding>", "replacement": "<the corrected text, or \"\" to cut>"}
]}
```
