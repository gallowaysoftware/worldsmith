You are a line editor stripping the machine-prose tells out of a passage of hard
science-fiction. Below is a JSON list of sentences pulled from a draft, each with the
reason it was flagged. Rewrite EACH sentence to fix its flaw and return one replacement
per sentence.

```json
{{ readFile .inputs.spans_file }}
```

Fix each sentence according to its `reason`:

- **repeated opener "X Y"** — too many sentences in the draft begin this same way. Recast
  so the sentence does NOT start with that stem. Lead with the verb, the object, a
  subordinate clause, a concrete noun, or a fragment. Change the STRUCTURE — don't just
  swap a synonym for the first word.
- **slop "term"** — the word is over-used LLM filler. Replace it with plainer, more
  specific language, or cut it, keeping the meaning. Avoid these tells generally too:
  shimmer, glint, thrum, pulse, cascade, tapestry, palpable, testament, symphony, myriad,
  ineffable, ethereal, inexorable, "for a moment", "in that instant".
- **not-X-but-Y cadence** — the antithesis frame ("not anger but something colder") is
  over-used. Recast as a direct statement of what IS, dropping the not/but scaffolding.

Hard rules — a replacement that breaks any of these is WORSE than the original:

- **Preserve meaning exactly.** Same facts, names, events, character knowledge, and
  tense (simple past). Do NOT add information, foreshadowing, sensory detail, or metaphor
  that wasn't already there — you are REMOVING ornament, not adding it.
- **Preserve length.** Stay within ~±20% of the original sentence's length. One sentence
  in, one sentence out — never merge or split.
- **Self-contained recast.** You see each sentence in isolation; leave pronouns and
  references exactly as they are so the rewrite drops back into place cleanly.
- **Match the register.** Spare, concrete, grounded hard-SF. No purple flourish.

Output ONLY this JSON. The `span` MUST be the flagged sentence copied VERBATIM (exactly as
given, so it can be located in the draft):

{"replacements": [
  {"span": "<the original sentence, exactly as given>", "replacement": "<your rewrite>"}
]}

If a sentence genuinely cannot be improved without changing its meaning, return it
unchanged (span identical to replacement) — it will be skipped.
