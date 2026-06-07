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

Write each license as ONE concrete sentence that names exactly what may be stated AND what
must still stay sealed (mirror the notebook's own wording). Cover every chapter number; a
chapter that reveals nothing gets `"reveals": []`.

# Output — JSON ONLY (first byte `{`, no prose, no commentary)

{"chapters": [
  {"n": 1, "reveals": ["<one license sentence>"]},
  {"n": 2, "reveals": []}
]}
