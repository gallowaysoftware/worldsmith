You are the showrunner planning ONE book of a serialised novel sequence. Draft this book's
ordered **chapter beats** — the chapter-by-chapter spine a writer will then turn into prose.
You are planning structure, not writing prose.

# The series

**{{ .inputs.series_title }}** — Book {{ .inputs.book_n }}{{ if .inputs.book_title }}: {{ .inputs.book_title }}{{ end }}

The whole-series arc (what the sequence must hit, and where it ends):

{{ .inputs.series_arc }}

What earlier books have already covered (continue forward from here; do NOT recap):

{{ .inputs.prior_books }}

# This book

**Premise:** {{ .inputs.premise }}

**The beats this book must cover (in rough order):**

{{ .inputs.arc_summary }}

Aim for about **{{ .inputs.target_chapters }}** chapters. Each chapter is one beat-cluster
of the book — a scene or two that moves the story a clear step — not the whole act.

# World bible (obey it — hard physics, no mysticism, the established tone)

{{ readFile .inputs.world_file }}

# Characters

```json
{{ readFile .inputs.characters_file }}
```

# Canon so far (what the reader already knows — state freely)

{{ readFile .inputs.canon_file }}

# The author's sealed notebook (what you KNOW; mostly NOT for the page)

{{ readFile .inputs.notebook_file }}

# What this book is LICENSED to reveal

{{ .inputs.reveals }}

The notebook holds the author's secrets. This book may put the **licensed** items above onto
the page (paced across its chapters); everything else in the notebook stays SEALED — it may
press on a scene as subtext but must not be stated or confirmed. Items a dossier marks NEVER
are never revealed.

# POV roster (assign each chapter a POV from this list)

{{ .inputs.pov_roster }}

---

# How to plan

- **Tell this book's story.** Cover its beats in a satisfying order, escalating to the book's
  climax, and leave the series one clear step further along toward its final state.
- **Assign a POV per chapter from the roster, and use it as the fog control.** A chapter is
  narrated by someone who only knows what that character would know — so route chapters that
  must stay clear of still-sealed material to a POV who *doesn't know* it (they cannot leak what
  they don't know). Reserve the knowing POVs (e.g. a captive who has learned a secret) for
  chapters where that secret is licensed or already canon. Vary POV across the ensemble for
  texture; don't head-hop within a chapter.
- **Pace the reveals.** Spread the licensed material across the book rather than dumping it in
  one chapter; keep sealed-unlicensed material to subtext.
- **Do NOT invent a canon character's fate, or contradict an established event.** Established
  people, ships, and events have fixed facts in the canon/notebook above — honour them exactly;
  do not assign a new outcome they don't already have. Do NOT have a named character captured,
  killed, freed, present, or revealed in a chapter unless canon places them there. In particular,
  do NOT conflate two distinct events into one: if canon says a character was taken in a separate,
  earlier event (e.g. Ila Vren, captured ~a year before aboard the *Meridian*, already in the
  Vault), then a *later* event must NOT also capture them — give the later event its own ship,
  its own outcome, and no captive canon doesn't record. When a beat from the summary is vague
  about who/which-ship, fill it with a NEW name consistent with canon, never by reusing a canon
  character/ship whose story is already fixed elsewhere. A chapter renders the planned step; it
  does not rewrite who-was-where.
- **Each chapter beat:** a `title`, a one-line `hook` (the reason to keep listening), 2–4
  `beats` (what happens), the `pov` (a roster slug), and any `constraints` (what NOT to do this
  chapter — e.g. a specific secret that stays sealed here).
- **The deeper dark stays honour-by-absence unless this book's reveals license it.** Do NOT
  build a chapter around the threat in the inter-arm dark / what the Vesh fear, and do NOT
  assign a POV whose nature would force it onto the page (a Vesh of the Listening senses the
  dark — narrating a not-yet-licensed-dark scene through one invites a leak). Route such scenes
  through a POV who does NOT sense the dark (a Vesh non-Listener, a human scout), and add a
  constraint: "the deeper dark is honour-by-absence here — formless dread only, never named,
  shown, or given agency/voice/response."
- Honour the bible: no magic/mysticism, hard mechanisms, established institutions and tone.

# Output — JSON ONLY

Return ONE JSON object, first byte `{`, no prose or markdown around it:

```json
{"chapters": [
  {"title": "<chapter title>", "hook": "<one line>", "beats": ["<what happens>", "..."], "pov": "<roster slug>", "constraints": ["<what NOT to do this chapter>"]}
]}
```
