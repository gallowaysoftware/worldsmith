You are planning the REVEAL PACING for one book of a series — deciding, secret by secret,
the chapter where each first reaches the page, so the reader learns things in the right
order instead of all at once. You are NOT writing prose; you are producing a pacing plan.

# The author's private notebook — the SECRETS to pace

{{ readFile .inputs.notebook_file }}

# The world (for context)

{{ readFile .inputs.world_file }}

# What Book {{ .inputs.book_n }} ({{ .inputs.book_title }}) is allowed to reveal AT ALL

{{ .inputs.book_reveals }}

Anything in the notebook NOT covered by this license stays sealed for the whole book (it
belongs to a later book) — never license it in any chapter here.

# The book's chapters — what each one dramatizes

{{ .inputs.chapters }}

---

# Your task

For EACH chapter, decide which sealed items (if any) it should put ON THE PAGE for the
first time — and only those. Principles:

- **Pace by content.** A secret first surfaces in the chapter whose events most naturally
  carry it — the POV who would realize it, the scene where it is shown — never earlier.
- **Reveal once, then it is canon.** After a chapter reveals something, later chapters may
  reference it freely. License each item in its FIRST chapter only; do not re-license it.
- **Default to sealed.** Most chapters reveal nothing new (`[]`). A reveal is the
  exception, earned by the chapter's content. When unsure, keep it sealed (subtext) — a
  premature reveal is exactly the failure this plan prevents.
- **Bound each reveal.** License the CORE of what surfaces, not its implications. If a
  chapter reveals a character's personal realization, do NOT also license the strategic
  scale behind it — that is a separate, later reveal.
- **Honour-by-absence is absolute.** Anything the notebook marks NEVER / honour-by-absence
  is never licensed, in any chapter.
- **A chapter's own `[constraint]` notes are BINDING.** They are the author's explicit
  intent for what that chapter must withhold. NEVER license anything a chapter's
  constraint says to keep sealed, keep implicit, or keep out of a POV's knowledge. If a
  constraint says "do not reveal X to the reader yet" or "show it only through Y's
  reaction" → that chapter does NOT license X (leave it sealed/subtext). If a constraint
  says a POV "does not know" something → do not license a reveal that requires that POV to
  know it. When a constraint and the book license seem to disagree, the constraint wins.

Write each license as a BOUNDED instruction with TWO explicit halves — this is the most
important rule, and the difference between a license that holds and one the writer escalates
through:

1. **May state:** the CORE that surfaces this chapter (one concrete thing).
2. **Must still withhold:** the adjacent escalations the writer will be tempted to reach
   for but that stay sealed — the scale, the count, the mechanism/methodology, the
   heritability, the strategic consequence, the cohort, the wider program. Name them
   explicitly so the writer knows the edge.

A license that only says what MAY be revealed (without naming what must stay sealed) WILL
leak — the writer dramatizes a personal reveal into its strategic implications. Always
write both halves.

Example of a properly bounded license (chapter where a captive realizes her situation):
"MAY state: the captive's own realization that she is kept alive to be harvested and bred
from, as biological stock — her personal status, in this room. MUST still withhold: that
other captives/products or a bred cohort exist, any count or scale, that the trait is
heritable or how the selection works, and the strategic war-consequence — all sealed for
later."

Cover every chapter number; a chapter that reveals nothing gets `"reveals": []`.

# Output — JSON ONLY (first byte `{`, no prose, no commentary)

{"chapters": [
  {"n": 1, "reveals": ["<one license sentence>"]},
  {"n": 2, "reveals": []}
]}
