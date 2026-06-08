You are a story editor adjusting ONE chapter's beats so its sealed material stays SUBTEXT.
The chapter currently dramatizes a secret too openly on the page; your job is to keep the
exact same plot events but move the secret off-page — shown through what characters do,
avoid, fear, or won't say, never stated outright. You are editing the OUTLINE (beats), not
writing prose.

# Chapter {{ .inputs.chapter_n }}: {{ .inputs.title }}

## Current beats

{{ .inputs.beats }}

## Current constraints

{{ .inputs.constraints }}

## This chapter's reveal-license (what it MAY put on the page)

{{ .inputs.reveals }}

## The author's private notebook (the sealed material — keep off-page unless licensed)

{{ readFile .inputs.notebook_file }}

## The world (for context)

{{ readFile .inputs.world_file }}

---

# Your task

Rewrite the beats so:

- **Same events, same plot.** Every beat's action still happens — who is present, what they
  do, where it goes. Do NOT cut the scene, change the outcome, or invent new plot. This is a
  framing edit, not a rewrite of the story.
- **Keep what the license permits; tone only the excess.** If the license has a "MAY state"
  clause, that reveal STAYS on the page — do NOT tone it away. Only the material the license
  marks "MUST withhold" (and anything sealed in the notebook the license doesn't permit) gets
  pushed to subtext. For a chapter whose license reveals nothing, tone ALL sealed material to
  subtext. The leak you are fixing is the *excess* beyond the license, not the licensed core.
- **Sealed material becomes subtext.** Where a beat dramatizes withheld/sealed material (the
  breeding programme's scale, the cohort, the heritability, the methodology, the strategic
  consequence), recast that beat so the secret is only *implied* — a guarded reaction, an
  evasion, a euphemism, a thing noticed but not named.
- **Strip the leaky specifics from the beat language itself.** A beat that says "Augustus
  boasts about the bred cohort" becomes "Augustus alludes to a coming advantage he will not
  name." A beat naming counts, batches, or "generations" loses those specifics.
- **Output a CLEAN, consistent constraint set.** Return the COMPLETE constraints for the
  chapter, not an append. DROP any existing constraint that now contradicts the license or the
  toning (e.g. an old "reveal X" when X is now sealed — that contradiction confuses the
  writer). Keep the constraints that still hold, and ADD one sealing constraint that names
  exactly what this chapter must keep sealed.

Keep each beat one sentence, in the same style as the input. Preserve the chapter's POV.

# Output — JSON ONLY (first byte `{`, no prose, no commentary)

{"hook": "<revised hook, or the original if it was fine>",
 "beats": ["<revised beat 1>", "<revised beat 2>", "..."],
 "constraints": ["<existing + a new sealing constraint naming what stays sealed>"]}
