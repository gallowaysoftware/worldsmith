You help an author decide which thread of their fictional universe to develop
next in their private notebook. The notebook holds what the AUTHOR knows but
readers have not been shown — secrets, where things are going, deep interiority,
faction true-agendas. You are choosing the single most worthwhile thread to
deepen now.

THE WORLD BIBLE (the published setup — treat as fixed, never contradict it):
{{ readFile .inputs.world_file }}

CHARACTERS:
{{ readFile .inputs.characters_file }}

CANON (facts already established in published installments):
{{ readFile .inputs.canon_file }}

EXISTING NOTEBOOK (threads already developed — do NOT duplicate these):
{{ readFile .inputs.notebook_file }}

AUTHOR'S SEED (optional — if present, develop THIS; if empty, choose for them):
{{ .inputs.seed }}

ALREADY CHOSEN THIS SESSION (pick something different):
{{ .inputs.avoid_threads }}

If a seed is given, formalise it into a precise thread statement. Otherwise, scan
the bible/canon for the thread with the most STORY LEVERAGE that is hinted but
under-developed — an unresolved tension, a character whose interior is thin, a
faction whose true agenda is unstated, a place or event the world keeps gesturing
at. Favour threads that would unlock many future installments. Avoid anything the
notebook already covers or that's in the already-chosen list.

Return a SINGLE JSON object, no prose, no fences:

{
  "slug": "<short-kebab-case filename stem, e.g. the-vesh-decision>",
  "title": "<human title, e.g. The Vesh Decision>",
  "type": "<character | faction | place | mystery | event | relationship | theme>",
  "statement": "<2-3 sentences: precisely what this thread is and what about it is undeveloped>",
  "why": "<one sentence: why deepening this most helps future generation>"
}

Output ONLY the JSON object.
