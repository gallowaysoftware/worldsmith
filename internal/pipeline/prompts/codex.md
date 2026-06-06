Compile a companion CODEX — a reader's encyclopedia — for this fictional world,
from its published bible and the canon established so far. It is the kind of
browsable reference a fan would keep open beside the book.

# The world bible

{{ readFile .inputs.world_file }}

# Characters

```json
{{ readFile .inputs.characters_file }}
```

# Canon established so far (events/facts from published installments)

{{ readFile .inputs.canon_file }}

---

Organise the codex into clear sections with `##` headings, choosing the ones that
fit this world (typical set):

- **Factions & Powers** — the organisations/nations and what each wants.
- **Key Figures** — the named cast and recurring figures; a few lines each.
- **Places** — worlds, cities, ships, institutions.
- **Concepts & Technology** — the systems, rules, and terms a reader must grasp.
- **Timeline** — the established events in order.

Within a section, one terse encyclopedic entry per item: a bold name, then a
couple of sentences. Cross-reference other entries with `[[Name]]` where it helps
a reader navigate.

SPOILER-SAFE — this is binding:
- Include ONLY what the bible and canon above establish. Do not speculate, do not
  invent, and do not reveal anything not yet shown to readers. If the bible marks
  something as a withheld mystery, the codex may name the mystery as an open
  question but must NOT answer it.
- Stay consistent with established names, dates, and facts exactly.

Output the codex as a single markdown document. Start with a `# <World Title> — Codex`
heading. No preamble, no commentary.
