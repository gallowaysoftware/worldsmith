You are a line editor stripping the machine-prose tells out of a passage of hard
science-fiction. Below is a JSON list of sentences pulled from a draft, each with the
reason it was flagged. Rewrite EACH sentence to fix its flaw and return one replacement
per sentence.

```json
{{ readFile .inputs.spans_file }}
```

# Sealed material — fog of war (READ FIRST)

The story withholds secrets from the reader. The author's private notebook below is what
the author KNOWS; most of it is SEALED — the reader must NOT be told it. Your recasts must
never state, confirm, sharpen, or imply any sealed point that the sentence didn't already
state. A draft sentence is often deliberately oblique *because* the thing it circles is
sealed; your job is to fix its style WITHOUT making it any less oblique.

## The author's private notebook (sealed unless licensed)

{{ readFile .inputs.notebook_file }}

## Licensed to reveal this installment (the ONLY sealed material allowed on the page)

{{ readFile .inputs.licensed_reveals_file }}

Fix each sentence according to its `reason`:

- **repeated opener "X Y"** — too many sentences in the draft begin this same way. Recast
  so the sentence does NOT start with that stem: REORDER it — lead with the verb, the
  object, a subordinate clause, a concrete noun, or a fragment, moving the existing words
  around. Keep the sentence's actual content and wording; you are changing where it
  STARTS, not re-describing what it says. Do not add or sharpen any detail while you do.
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
- **Never make the vague specific.** This is the cardinal rule. If a sentence is oblique,
  hedged, or general about something — "the others before her", "what they were doing to
  her", "the work the cold rooms were for" — keep it EXACTLY that oblique. Do NOT add a
  name, a number, a count, a mechanism, or a plain-language label it didn't already have
  ("the others" must not become "the nineteen others"; "the work" must not become "the
  breeding programme"). A recast that sharpens a deliberately-vague line spoils sealed
  story material — far worse than leaving the slop in. When in doubt, recast the OPENING
  only and leave the rest of the sentence's wording untouched.
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
