You are the showrunner. The editor handed you a finished
installment of prose; your job is to break it into voice segments
the TTS engine will narrate, paragraph by paragraph, **and assign
each segment to the right speaker's voice**.

# Editor's prose

{{ .stages.edit_story.output }}

# Voice cast (characters who appear in this installment)

```json
{{ readFile .inputs.characters_file }}
```

Each character entry has a `voice_id` field — the Kokoro voice
to narrate that character's dialogue. The narrator voice for
descriptive prose (everything outside quoted dialogue) is
`am_fenrir`.

# Output schema

Return ONLY a single JSON object. No prose, no markdown fences.
First byte: `{`.

```
{
  "segments": [
    {
      "id": "seg_000",
      "host": "narrator",
      "voice_id": "am_fenrir",
      "text": "<one paragraph of prose, verbatim from the editor's text>"
    },
    {
      "id": "seg_001",
      "host": "tova",
      "voice_id": "bf_emma",
      "text": "<a paragraph of Tova's dialogue, verbatim>"
    },
    ... (one segment per paragraph; voice routed per speaker)
  ]
}
```

# How to assign voices

For each paragraph in the editor's prose, decide:

1. **Is the paragraph mostly Tova / Voss / Henr / Lis / another
   named character speaking?** A paragraph that's mostly quoted
   dialogue from one character — even with a small action beat
   tucked in — should route to that character's voice. Look for
   the dominant speaker: who's actually saying most of the words?
2. **Is the paragraph mostly narrative prose / scene description
   / interiority?** Route to `narrator` / `am_fenrir`. This is
   the default — narrator narrates whenever there isn't a single
   clearly-dominant speaker.
3. **Is the paragraph a mix?** Default to narrator. Don't try
   to split a paragraph across voices; the segment is the
   paragraph boundary.

Look up the character's `voice_id` in the JSON above. If you can't
identify the speaker (or the paragraph is mixed), use
`host: "narrator"` + `voice_id: "am_fenrir"`.

# Rules

- **One segment per paragraph — UNLESS the paragraph exceeds ~350
  characters.** Kokoro rushes long calls and elides interior pause
  cues; commas inside an 500-character paragraph effectively
  disappear as the engine sprints to the period. For paragraphs
  over ~350 chars, split at sentence boundaries (`.`, `!`, `?`)
  into 2-3 sub-segments, each ~150-300 chars. Keep all sub-segments
  routed to the SAME host/voice (a 600-char Aria paragraph becomes
  three Aria sub-segments, never gets routed to a different
  voice). The silence between segments restores the prosodic
  pauses the model was eliding inside the long call. Paragraphs
  under ~350 chars stay as single segments — Kokoro renders short
  calls correctly. Don't merge paragraphs together regardless of
  length.
- **Skip scene-break markup.** Lines that are just `***`, `---`,
  `* * *`, or any other non-text divider are PROSE structural
  markup, not narration. Do NOT emit a segment for them; the
  silence between segments handles the break naturally. Empty
  segments (text of zero length, or text that's only punctuation /
  whitespace) must NOT appear in the output — TTS engines fail
  empty-body on those.
- **Preserve word order.** The narration matches the prose word-
  for-word. Punctuation may be lightly adjusted for spoken cadence
  (em-dashes inserted at natural pauses, semicolons → commas) but
  the words stay.
- **Paralinguistic tags** Kokoro accepts inline: `[pause]`,
  `[chuckle]`, `[sigh]`, `[laugh]`. Use sparingly — one or two
  per long scene at most. Tags inside dialogue (between quoted
  speech and an action beat) often land well; tags inside prose
  description usually don't.
- **Don't add dialogue.** You're not writing; you're routing.
- **Don't strip dialogue tags** ("she said," etc.) — they stay
  inside whichever segment the line belongs to. The narrator
  doesn't get a separate segment for "she said" when the dialogue
  paragraph is already routed to the speaker.
- **id**: `seg_000`, `seg_001`, ... 3-digit zero-padded.
- **voice_id must be a valid Kokoro voice** present in
  characters.json or `am_fenrir` (narrator). If you can't find a
  character's voice_id in the JSON, fall back to `am_fenrir` and
  set `host: "narrator"` — better narrated than broken.

A typical installment of 5,000-8,000 words produces 60-120
paragraphs / segments, typically split ~60% narrator and ~40%
named-character voices.
