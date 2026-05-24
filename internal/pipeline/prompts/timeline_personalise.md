You are completing the timeline. The eras, global/regional anchors,
and regional consequences are below. Your job is to weave the named
characters of this world into recorded history — give the named cast
**ancestors, mentors, predecessors, dead siblings, exiled family**
that anchor them to specific historical events.

# World bible

{{ readFile .inputs.world_file }}

# Characters

```json
{{ readFile .inputs.characters_file }}
```

# Eras

```json
{{ .stages.seed_eras.output }}
```

# Anchors

```json
{{ .stages.seed_anchors.output }}
```

# Regional consequences

```json
{{ .stages.elaborate_regional.output }}
```

---

# Your task

Propose **personal-scale events** that connect the named characters
to the broader history. Each character should pick up **1–3 anchoring
events** in their family's past — not the character's own birth/death,
which the prose will handle, but events their parents / mentors /
factions did, that the character either inherits or rebels against.

Guidelines:

- An event must reference **at least one named character** in
  `actors`, and at minimum one **prior event** (anchor or regional
  consequence) in `caused_by` so the personal scale is tied to
  history, not floating in the void.
- Scope is almost always `personal` (named-individual events) or
  `local` (a household/order/farmstead-level event).
- Era: the era the event happens in, NOT necessarily the era the
  character lives in. Most events here will be in the era
  immediately before the character's life, since they're
  setting up the character's inheritance/grievance.
- Mix tones: a master's death that left a debt; an oath sworn at a
  ruined shrine; a sibling lost during a regional war; a marriage
  alliance that fell through; an artifact taken from a fallen
  monarch. Don't make every event tragic — quiet ones are texture
  too.
- ID numbering: continue the monotonic sequence past the largest
  id in the regional-consequences output.
- DO NOT set `visibility` (the fog pass handles that).
- DO NOT echo events from prior passes.

# Output

Strict JSON, no preamble, no commentary. Schema:

```json
{
  "events": [
    {
      "id": "evt_0055",
      "year": 410,
      "era": "salt-wars",
      "kind": "death",
      "scope": "personal",
      "region": "veld",
      "actors": ["asha", "tomir-vasi"],
      "summary": "Tomir Vasi, Asha's master and the last living engineer of the Drowning, dies of lungrot in the silver-bone quarter, leaving Asha his salvaged plans and a debt to the Coastal Guild.",
      "consequences": [],
      "caused_by": ["evt_0027", "evt_0030"]
    }
  ]
}
```

First byte of output: `{`. Last byte: `}`. No markdown fences.
