You are a worldbuilder writing the bones of a fictional world's
recorded history. The world bible below is the human author's
source of truth — read it and produce a list of named eras that
will hang every later historical event off a tonal/political
backbone.

# World bible

{{ readFile .inputs.world_file }}

# Existing timeline (events already canonical in this world)

{{ readFileOrEmpty .inputs.existing_timeline_file }}

---

# Your task

Produce **3–7 named eras** spanning the world's recorded history.
Each era must:

1. Have a memorable, in-world **name** (`"The Long Quiet"`, `"The
   Salt Wars"`, `"The Coronation Years"`) — not a generic
   calendar tag.
2. Have a **slug** for filesystem use (lowercase, hyphenated).
3. Have a year range `start` / `end` (integers, may be negative for
   pre-epoch). The earliest era's `start` defines the world's
   recorded-history horizon; the latest era's `end` should be at
   or just past the world's current "now" if the bible implies one.
4. Have a one-sentence `summary` capturing the era's **tonal and
   political shape** — what was the texture of life, who held
   power, what was the dominant anxiety. Avoid event-specifics
   here; the next pass invents those.

If an existing timeline is provided above, your new eras must NOT
contradict the years/names already in use. Either reuse the existing
era anchors verbatim or extend them at either end of the timeline.

# Output

Strict JSON, no preamble, no commentary. Schema:

```json
{
  "eras": [
    {"slug": "long-quiet", "name": "The Long Quiet", "start": 1, "end": 380, "summary": "..."}
  ]
}
```

First byte of output: `{`. Last byte: `}`. No markdown fences.
