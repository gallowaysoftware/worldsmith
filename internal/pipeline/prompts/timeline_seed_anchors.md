You are continuing the worldbuilding. The eras backbone is below;
for each era, propose the **anchor events** — the foundings, wars,
schisms, plagues, discoveries, and treaties around which everything
else in that era turns. Later passes will hang regional and
personal-scale events off these anchors, so anchors should be
specific enough to support consequences but general enough to leave
narrative room.

# World bible

{{ readFile .inputs.world_file }}

# Eras (output of the previous pass)

```json
{{ .stages.seed_eras.output }}
```

# Existing timeline (events already canonical in this world)

{{ if ne .inputs.existing_timeline_file "" }}{{ readFile .inputs.existing_timeline_file }}{{ else }}(none — generating from scratch){{ end }}

---

# Your task

For each era in the list above, propose **3–8 anchor events**.
Distribute events through the era's year range; don't cluster them
all at the start or end. Mix kinds — a sequence of 8 wars in a row
would be lifeless. Aim across:

- `war` / `battle`
- `founding` (city, order, faith, university)
- `treaty` / `schism`
- `discovery` / `invention` (technological, magical, geographic)
- `disaster` (plague, eruption, famine)
- `coronation` / `betrayal` (when the world bible implies named
  monarchies / dynasties)
- `prophecy` / `miracle` (when the world bible implies religion or
  high-strangeness)

Every event must have:

- **`id`**: globally unique, format `evt_NNNN` (numbering starts at
  `evt_0001` and is monotonic across this run). If the existing
  timeline shows ids like `evt_0042` are already in use, start
  numbering from one higher.
- **`year`**: integer within its era's start/end range.
- **`era`**: the slug of the era it belongs to.
- **`kind`**: one of the categories above.
- **`scope`**: `global` (changes the whole world) or `regional`
  (changes a named territory). All anchor events are `global` or
  `regional`; later passes handle `local`/`personal`.
- **`region`**: required when `scope` is `regional`; cite a place
  from the world bible.
- **`actors`**: 1–4 in-world entities involved. Use slugs
  (`marsh-coalition`, `veld-magistracy`). Invent if the bible
  doesn't name them, but the names should sit naturally in the
  bible's tone.
- **`summary`**: one sentence, plain past tense, names the actors,
  states what happened. Do not editorialise.
- **`consequences`**: empty list `[]` (later passes fill this in).

Do NOT set `visibility` — the fog pass handles that.
Do NOT set `confidence` or `source` — the CLI stamps those.

# Output

Strict JSON, no preamble, no commentary. Schema:

```json
{
  "events": [
    {
      "id": "evt_0001",
      "year": 230,
      "era": "long-quiet",
      "kind": "founding",
      "scope": "regional",
      "region": "vahn-reach",
      "actors": ["coastal-guild"],
      "summary": "The Coastal Guild lights the first salt-lamp at Vahn's Reach, formalising the harbour's transition from fishing village to trading port.",
      "consequences": []
    }
  ]
}
```

First byte of output: `{`. Last byte: `}`. No markdown fences.
