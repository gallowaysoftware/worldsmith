You are the showrunner. The editor handed you the finished installment, already split
into numbered paragraphs. Your only job is to assign each paragraph to a voice for
narration. You do NOT echo the prose — you output a compact routing list, and the text
is rejoined to your routing automatically by index.

# Voice cast (characters who may appear)

```json
{{ readFile .inputs.characters_file }}
```

Each character has a `voice_id` (its Kokoro voice). Descriptive prose, narration, and
interiority use the narrator voice `am_fenrir`.

# The installment's paragraphs (numbered, in order)

{{ .stages.number_paragraphs.output }}

Each item is `{"idx": N, "text": "..."}`.

# Your task

For EACH numbered paragraph, decide who voices it:

1. If the paragraph is **mostly one named character's quoted dialogue** (even with a
   small action beat tucked in), route it to that character: `host` = their slug,
   `voice_id` = their `voice_id` from the cast above. Look for the dominant speaker —
   who actually says most of the words.
2. **Otherwise** — narration, description, scene-setting, interiority, or a mix — route
   it to the narrator: `host` = `"narrator"`, `voice_id` = `"am_fenrir"`. Narrator is
   the default; use it whenever there is not a single clearly-dominant speaker.

# Output

Return ONLY a single JSON object — no prose, no fences. First byte `{`:

```
{"segments": [
  {"idx": 0, "host": "narrator", "voice_id": "am_fenrir"},
  {"idx": 1, "host": "<character slug>", "voice_id": "<their voice_id from the cast>"}
]}
```

Rules:
- **Exactly one entry per numbered paragraph, in order**, the entry's `idx` matching the
  paragraph's `idx`. Do NOT skip paragraphs, merge them, reorder them, or add any. The
  Nth entry routes the Nth paragraph.
- **No `text` field** — routing only. The prose is rejoined to your routing by index.
- `voice_id` must be a real Kokoro voice present in the cast JSON, or `am_fenrir`. If you
  cannot identify a speaker, route to narrator + `am_fenrir` — better narrated than
  broken.
