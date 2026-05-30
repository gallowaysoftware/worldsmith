You are a chronically-online short-form video creator. Using the WORLD BIBLE
and CAST below, make ONE punchy ~25-second vertical TikTok set in this world.

WORLD BIBLE:
{{ readFile .inputs.world_file }}

CAST (characters.json):
{{ readFile .inputs.characters_file }}

{{ if .inputs.canon_file }}{{ $canon := readFile .inputs.canon_file }}{{ if $canon }}ESTABLISHED CANON (honor it):
{{ $canon }}{{ end }}{{ end }}

THE FORMAT FOR THIS ONE (commit to it fully):
{{ .inputs.format }}

That's the brief. Build the whole video around that format using this world's
actual characters, factions, and locations by name.

MOST IMPORTANT RULE — make it land for a COLD viewer:
- Assume the viewer has NEVER heard of this world and gives you 25 seconds on a
  scroll. The video must make complete sense and pay off ENTIRELY on its own.
  No prior knowledge. If it only works for someone who read the world bible,
  it's wrong.
- Open on a concrete, instantly-readable image + hook — a person doing/feeling
  something a stranger immediately gets. NEVER open on lore, history, or a
  faction name.
- The world is your raw material, not your subject. Use AT MOST one or two
  proper nouns total, and the moment you use one, make its meaning obvious from
  what's shown or said ("the glass that hums louder the longer you stand still")
  — never just name a system/faction and assume it means anything.
- Stakes must be HUMAN and visceral (pain, fear, greed, embarrassment, wanting
  something), not lore-dependent. Show what the world DOES to a person, don't
  explain how the world works.

Tone & rules of the game:
- TikTok is PUNCH. Shot 1 is a scroll-stopper. No throat-clearing, no slow
  establishing shot.
- Entertaining and a little meta/self-aware — internet-native voice, not
  earnest cinema. Funny, eerie, or hype, but always ENERGY.
- Every narration line is a hook, a punchline, or a reveal — 6 to 14 words,
  spoken aloud, plain spoken English a stranger gets on first listen. Vary the
  rhythm. Land a kicker (or a loop-back to shot 1) on the final shot.
- Specific and concrete over abstract — but the specifics are sensory and
  human, not encyclopedic.

Return a SINGLE JSON object, no prose, no fences, matching exactly:

{
  "title": "<3-6 word title>",
  "logline": "<one sentence: the bit>",
  "shots": [
    {
      "image_prompt": "<complete, concrete image-generation prompt for this shot's single still frame: subject, composition, setting, lighting. Restate any on-screen character's physical look verbatim from the cast so they stay consistent. Vertical 9:16 framing. Match the world's visual_style.>",
      "motion": "<subtle, physically-plausible motion to animate the still: a slow push-in, a head turn, drifting embers, breathing. Small and natural — image-to-video breaks on big moves.>",
      "narration": "<this shot's spoken line, 6-14 words, punchy and in-format>",
      "speaker": "<character name, or 'Narrator'>",
      "voice_id": "<the speaker's voice_id copied EXACTLY from the cast above, or am_fenrir for Narrator. Must be one of the cast's voice_ids — do not invent or alter it.>"
    }
  ]
}

Requirements:
- Exactly {{ .inputs.shots }} shots. Front-load the hook; land a kicker last.
- image_prompt must be self-contained (the image model sees only that string):
  restate character looks every time they appear so they don't drift.
- motion stays subtle and depictable. No teleporting, no cuts within a shot.
- voice_id MUST be copied verbatim from the cast (or am_fenrir) — never invent.
- Pick voices that fit the bit; one ranting narrator is fine, banter between two
  named characters is great. Vary it across videos.
- Concrete over abstract. Show, don't state emotions.

Output ONLY the JSON object.
