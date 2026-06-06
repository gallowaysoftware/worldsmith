You are the keeper of the canon. A draft list of this installment's new canon
facts has been extracted from the prose — but extraction drifts. It can record a
version of events the finished prose does not support (a character "killed" who
in fact survives), a mechanism an edit already replaced, a person or place the
prose never named, or a fact that contradicts what was already canon. Your job is
to produce the CORRECTED canon delta: every fact true to the FINISHED PROSE and
consistent with the established world.

# The finished prose — the SOURCE OF TRUTH for what happened

{{ .stages.edit_story.output }}

# The installment summary — the delta must agree with this

{{ .stages.summarize.output }}

# The draft canon delta — AUDIT AND CORRECT THIS

{{ .stages.canon_delta.output }}

# Existing canon — new facts only; never contradict a prior-installment fact

{{ readFile .inputs.canon_file }}

# World bible + characters — no fact may contradict these; no invented cast

{{ readFile .inputs.world_file }}

```json
{{ readFile .inputs.characters_file }}
```

# The author's sealed notebook — canon must NOT expose what this does not license

{{ readFile .inputs.notebook_file }}

# Licensed to reveal this installment (everything else in the notebook stays sealed)

{{ readFile .inputs.licensed_reveals_file }}

---

Produce the corrected canon delta. Rules:

- **The prose wins.** Every fact must be supported by the finished prose above. If
  the draft delta says X but the prose shows Y — a character recorded as *dead*
  who is alive at the end of the scene, an effect recorded by a mechanism the prose
  doesn't use — fix the fact to match the prose. (This is the exact failure that
  poisons later installments; hunt it.)
- **Agree with the summary.** The delta and the summary describe the same events;
  if they diverge, make the delta match the prose (and thus the summary).
- **No invented entities.** Do not record a character, place, ship, or thing the
  prose did not name. If the draft introduced a name that collides with or
  contradicts the bible/characters (e.g. a second person in a role the cast file
  already assigns to someone), drop it or correct it to the established cast.
- **No contradiction of existing canon.** A new fact that conflicts with a
  prior-installment fact is an error — drop it or reconcile it to the established
  truth. Record only what is genuinely NEW this installment.
- **Mechanisms obey the world's hard rules.** Record an effect by its true,
  rules-faithful mechanism (what the prose actually shows), never by a cheat.
- **Canon never canonizes a sealed secret.** Canon is reader-facing — what readers may
  carry forward. If the prose LEAKED sealed notebook material the licensed-reveals list
  does not permit, do NOT record it: drop the fact. Anything a dossier marks NEVER /
  "honour by absence" (the deepest secrets — the threat in the dark, whatever the Vesh
  fear) is never canon under any circumstance, even if the prose stated it. A leak that
  becomes canon is a permanent spoiler; refuse it here. (Licensed reveals above DO
  become canon — they are now reader-facing.)
- **Keep the draft's format** — the same markdown headings and terse, atomic
  bullet structure. Facts the next installment can rely on, nothing more.

Output ONLY the corrected canon delta markdown — no preamble, no notes about what
you changed.
