You are extending the timeline. The eras and global/regional anchors
are below. Your job is to invent the **regional consequences** of
those anchors — the rebellions, exile populations, founded guilds,
heretical schisms, abandoned cities, and minor wars that flow as
second-order effects from the big events of the era.

# World bible

{{ readFile .inputs.world_file }}

# Eras

```json
{{ .stages.seed_eras.output }}
```

# Anchor events (output of the prior pass)

```json
{{ .stages.seed_anchors.output }}
```

---

# Your task

Propose **2–4 regional consequence events per major anchor** (you can
skip minor anchors). Each consequence event must:

- Be a plausible second-order effect of one or more anchors above —
  not a parallel invention. Things like "the exiles from the
  Drowning founded the silver-bone refugee quarter in Veld" or
  "the harbour-blockade survivors formed the first navigators'
  guild."
- Cite the parent anchor(s) by `id` in a new `caused_by` field on
  the event.
- Have `scope: "regional"` or `scope: "local"`.
- Have a `region` (always required at these scopes).
- Be set in the **same era or the era immediately after** the
  parent anchor. Don't trail a consequence 300 years behind its
  cause unless the world bible makes that natural.
- Follow the same JSON shape as the anchor events: `id` (continue
  the numeric sequence; the next id should be one higher than the
  largest id in the anchor list), `year`, `era`, `kind`, `scope`,
  `region`, `actors`, `summary`, `consequences: []`. Add
  `caused_by` with the list of parent anchor ids.

Variety matters — don't make every consequence a war or a refugee
movement. Mix in: schisms, founded orders, mass migrations, lost
arts, plagues triggered by displacement, new guilds, abandoned
shrines, name changes of places.

Do NOT set `visibility` — the fog pass handles that.
Do NOT echo back the anchor events — only emit the new consequence
events.

# Output

Strict JSON, no preamble, no commentary. Schema:

```json
{
  "events": [
    {
      "id": "evt_0030",
      "year": 415,
      "era": "salt-wars",
      "kind": "founding",
      "scope": "regional",
      "region": "veld",
      "actors": ["silver-bone-quarter"],
      "summary": "Exiles displaced by the Drowning of the Iron Bridge settle on the eastern hill above Veld and found the silver-bone refugee quarter.",
      "consequences": [],
      "caused_by": ["evt_0027"]
    }
  ]
}
```

First byte of output: `{`. Last byte: `}`. No markdown fences.
