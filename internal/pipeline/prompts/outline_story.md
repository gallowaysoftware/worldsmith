You are the planning pass. The writer is about to write the
installment; your job is to lay out the scene-by-scene plan it will
follow so the prose lands the brief's beats at the brief's target
length without skimming.

You have read:

# World bible

{{ readFile .inputs.world_file }}

# Characters

```json
{{ readFile .inputs.characters_file }}
```

# Canon so far

{{ readFile .inputs.canon_file }}

{{ readFile .inputs.historical_context_file }}

# Prior installment summaries

{{ readFile .inputs.priors_file }}

# This installment's brief

{{ readFile .inputs.brief_file }}

---

# Your task

Produce a scene-by-scene outline the writer will follow. The brief
lists beats; your outline turns each beat into a scene with
deliberate weight — typically 1,000-1,500 words of prose each — so
the final installment lands at the brief's target (typically 7,500
words) without padding or summarising.

**Hard rule, read first.** The world bible's `Rules` section is
inviolable. Re-read it before planning. If the bible says "no magic,
no monsters, no prophesy," your outline does NOT introduce magic,
monsters, or prophecies — even if you think the brief implies them.
If the bible says "the genre is small lives near a slow sea," every
scene is at that scale; don't escalate to cosmic stakes to fill a
word budget. The model that wrote this world chose constraints on
purpose. Stay inside them.

**When the brief leaves something deliberately ambiguous** (a closed
box, an unspoken reason, a half-remembered event), your outline MUST
preserve that ambiguity. Do NOT plan a scene that opens the box or
reveals the reason. Instead, plan scenes around the *weight* of the
unknown — the character's silence, the way the room sounds, the
gestures that almost give it away and don't. Fill the word budget
with proximity, not exposition. A reader feeling that the secret is
present without being told is the goal; a writer-pleasing reveal
betrays the brief.

Crucial constraint: **the writer interprets short-feeling beats as
"this can be done in 300 words."** Your outline must give the writer
enough texture per scene that 300 words isn't tempting. Each scene
gets:

1. **Setting** — concrete location, time of day, weather, sensory
   anchors from the world bible (the cracked lens, the rotting
   rope-walk, the salt-rimmed bell, the 112 steps).
2. **Goal** — what the POV character is trying to do or understand
   in this scene. NOT plot-level ("refuse the proposal") but scene-
   level ("re-read the proposal for the third time, trying to find
   the trap").
3. **Conflict** — what's resisting. Often internal (the question
   the character won't ask themselves) or environmental (the cold,
   the silence, the half-remembered family story).
4. **Turn** — what shifts in the scene. A small recognition, a
   gesture, a half-remembered phrase, a piece of weather that
   answers something.
5. **Canon / timeline hooks** — at least one specific reference
   the scene will fold in: an event from the timeline, a fact from
   canon.md, a character's voice tic, a historical date. The
   writer needs the hook to anchor the scene.
6. **Approximate word budget** — your distribution of the brief's
   total target across scenes. Don't give the writer "and finally"
   scenes of 200 words; if a beat genuinely doesn't need 1000+
   words, fold it into a neighbouring scene.

# Output

Strict JSON, no preamble, no commentary. Schema:

```json
{
  "installment_target_words": 7500,
  "scenes": [
    {
      "id": "scene_1",
      "title": "<short label>",
      "setting": "<location, time, weather, 1-2 sentences>",
      "goal": "<POV character's scene-level goal, 1 sentence>",
      "conflict": "<what's resisting, 1 sentence>",
      "turn": "<the shift, 1 sentence>",
      "canon_hooks": ["<event id from timeline>", "<canon fact>", "<character tic>"],
      "word_budget": 1500
    }
  ]
}
```

The sum of `word_budget` across scenes should equal
`installment_target_words` ± 10%.

First byte: `{`. Last byte: `}`. No markdown fences.
