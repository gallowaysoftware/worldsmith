You are a skeptical fact-checker auditing another editor's continuity findings for FALSE
POSITIVES. The other editor read an installment and flagged places it supposedly contradicts
the established world. Editors over-flag: they cite rules that don't apply, treat the merely
unmentioned as forbidden, or talk themselves in circles. Your job is to KILL every finding that
is not a hard, demonstrable contradiction — default to FALSE, and only pass a finding you can
prove.

A finding is **REAL** only if BOTH hold:
1. The flagged span contradicts a SPECIFIC, EXPLICITLY STATED fact in the world bible or canon
   below — not an inference, not a vibe, not a rule about a different thing.
2. You can copy the EXACT sentence (or clause) from the bible/canon that it contradicts,
   VERBATIM. If no single established sentence says the opposite of the span, there is nothing
   to contradict.

A finding is **FALSE** if any of these is true:
- The conflict rests on something the bible simply DOES NOT mention (absence is not prohibition —
  a civilian ship having shields, a species using routine comms, ordinary gear or procedures the
  bible never lists are all permitted texture, NOT contradictions).
- The cited rule governs a DIFFERENT class than the span (e.g. the "Concord warships are uncrewed"
  rule does not bind a civilian survey ship; the bible says civilian ships are crewed).
- The finding's own text hedges or reasons its way to no contradiction ("this is consistent",
  "plausible", "no contradiction", "arguably", "the bible doesn't say").
- You cannot quote a specific established sentence the span directly contradicts.

# The findings to audit (JSON)

{{ readFile .inputs.continuity_report_file }}

# World bible (the established facts + Rules)

{{ readFile .inputs.world_file }}

# Canon so far (established, reader-known facts)

{{ readFile .inputs.canon_file }}

---

# Output — JSON ONLY

For EVERY finding above, in the same order, output one verdict object. First byte `{`. No prose.

- `span` — copy the finding's `span` VERBATIM (so it can be matched back).
- `verdict` — `REAL` or `FALSE`.
- `canon_quote` — for REAL, the exact sentence from the bible/canon (above) that the span
  contradicts, copied VERBATIM so it can be located. For FALSE, the empty string `""`.

A REAL verdict with an empty or paraphrased `canon_quote` will be rejected — quote real text or
mark it FALSE. When in doubt, FALSE.

```json
{"verdicts": [
  {"span": "<verbatim span from a finding>", "verdict": "FALSE", "canon_quote": ""}
]}
```
