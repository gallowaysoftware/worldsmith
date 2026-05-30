You are a short-form video director. Using the WORLD BIBLE and CAST below,
invent ONE specific, self-contained ~30-second vertical video scene set in
this world and break it into shots.

WORLD BIBLE:
{{ readFile .inputs.world_file }}

CAST (characters.json):
{{ readFile .inputs.characters_file }}

{{ if .inputs.canon_file }}{{ $canon := readFile .inputs.canon_file }}{{ if $canon }}ESTABLISHED CANON (honor it):
{{ $canon }}{{ end }}{{ end }}

Pick an interesting moment — a confrontation, a discovery, a small turn —
featuring 1-3 of the cast in one or two locations. It must read as a
complete beat with a hook at the start and a button at the end.

Return a SINGLE JSON object, no prose, no fences, matching exactly:

{
  "title": "<3-6 word scene title>",
  "logline": "<one sentence>",
  "shots": [
    {
      "image_prompt": "<a complete, concrete image-generation prompt for this shot's single still frame: subject, composition, setting, lighting. Include any on-screen character's physical look verbatim from the cast so they stay consistent. Vertical 9:16 framing.>",
      "motion": "<short description of the gentle motion to animate from the still: e.g. 'slow push-in as she turns her head', 'embers drift, cloak ripples'. Keep it subtle and physically plausible.>",
      "narration": "<the voiceover line for this shot, 10-18 words, spoken aloud — vivid, in the world's tone>",
      "speaker": "<character name, or 'Narrator'>",
      "voice_id": "<the speaker's voice_id from the cast, or am_fenrir for Narrator>"
    }
  ]
}

Requirements:
- Exactly {{ .inputs.shots }} shots.
- Each narration is ONE sentence, 10-18 words — short enough to land in ~4
  seconds of speech. The whole scene's narration should tell a tiny story.
- image_prompt must be self-contained (an image model sees only this string):
  restate the character look every time they appear so they don't drift.
- motion must be subtle and depictable (image-to-video works best with small,
  natural movement — drifting, turning, breathing, light changes). No
  teleporting, no scene cuts within a shot.
- Keep continuity: same characters look the same across shots; the location
  is consistent unless the narration moves us.
- Concrete and grounded; no vague mysticism, no narration that just names an
  emotion ("she felt fear") — show it.

Output ONLY the JSON object.
