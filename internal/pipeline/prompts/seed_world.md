You are a worldbuilding director. From the THEME below, invent ONE vivid,
internally-consistent fictional world for short-form video storytelling.

THEME: {{ .inputs.theme }}

Return a SINGLE JSON object, no prose, no markdown fences, matching exactly:

{
  "name": "<evocative 1-4 word world name>",
  "logline": "<one sentence: what makes this world worth watching>",
  "setting": "<2-3 sentences: where/when, the physical and social texture>",
  "history": "<2-3 sentences: the 1-2 past events that shape the present>",
  "tone": "<short phrase, e.g. 'melancholy solarpunk', 'baroque cosmic horror'>",
  "visual_style": "<a reusable image-generation style suffix: medium, lighting, palette, lens — applied to every shot for visual consistency>",
  "factions": ["<3-4 forces/groups pulling at the world>"],
  "rules": ["<2-4 hard constraints: what is possible/impossible here>"],
  "characters": [
    {
      "name": "<name>",
      "role": "<protagonist | foil | mentor | antagonist | witness>",
      "look": "<concrete physical description for image generation: age, build, features, clothing, distinguishing marks>",
      "voice_id": "<one of: am_fenrir, am_michael, am_puck, am_adam, am_eric, af_bella, af_nicole, bf_emma>",
      "personality": "<2-3 traits + what they want>"
    }
  ],
  "locations": [
    {
      "name": "<place name>",
      "description": "<what happens here, why it matters>",
      "look": "<concrete visual description for image generation>"
    }
  ]
}

Requirements:
- 4 to 6 characters; assign each a DISTINCT voice_id from the list (match
  apparent gender/age where it helps; reuse only if you run out).
- 3 to 4 locations.
- Every "look" and "visual_style" field must be CONCRETE and image-ready:
  nameable subjects, materials, lighting, colour — never abstract mood words
  alone. An image model must be able to draw it.
- Ground everything physical. No objects with agency, no vague mysticism;
  if something is strange, make it strange in a specific, depictable way.
- Keep it coherent: characters, factions, and locations should reference the
  same setting and rules.

Output ONLY the JSON object.
