You are the **fog of war** pass. Every event invented so far is a
naked fact; your job is to decide who in the world *knows* each
event, and what the public-told version is for those who don't.
Fog is what gives the prose layers — characters can be wrong about
history; rumour can carry weight; secrets can drive plot.

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

# Personal events

```json
{{ .stages.personalise.output }}
```

---

# Your task

For **every event** across all three prior outputs (anchors +
regional + personal), assign a `visibility` block:

```json
{
  "tier": "common|regional|cloistered|secret|lost",
  "known_to": ["actor-slug", "..."],
  "rumoured_as": "the public-told distortion, when applicable",
  "true_facts_hidden": ["the bits the rumour leaves out"]
}
```

## Tier semantics

- **`common`**: every literate person in the world knows it; the
  prose can reference it freely. Use for foundings of major
  cities, crowning of widely-known monarchs, plagues that swept a
  continent. Set `known_to: []` (everyone knows). Set
  `rumoured_as: ""`.

- **`regional`**: known within `event.region`; people from
  elsewhere have at most a vague rumour. Use for regional wars,
  local heroes, founding of a guild that didn't grow famous. Set
  `known_to: []` (the region's residents know automatically).
  Set `rumoured_as` to the public-told distortion non-region
  observers might have heard. If the event is regional but
  genuinely had no echo outside its region, leave `rumoured_as: ""`
  and non-region observers will see nothing.

- **`cloistered`**: known only to a named actor/faction allowlist.
  Use for secret oaths, hidden marriages, faked deaths, suppressed
  prophecies, true reasons behind public decisions. Set `known_to`
  to the slugs of who knows the truth. Set `rumoured_as` to the
  public version that non-knowers got.

- **`secret`**: NEVER spoken aloud, never shown to the writer
  prompt; surfaces only to the showrunner as a potential
  dramatic-reveal hint. Use sparingly — true conspiracies and
  mysteries that should pay off across multiple installments.
  Set `known_to: []` and `rumoured_as: ""`. The "true facts" are
  what the showrunner may eventually leak.

- **`lost`**: not in living memory; surfaces only when the human
  author writes it into a brief. Use for events deep in
  pre-history, civilisations whose name is forgotten, technology
  that was never re-invented. Set `known_to: []` and
  `rumoured_as: ""`.

## Distribution discipline

Don't make everything cloistered or secret — boring + monotone.
Sane targets for a 40-event timeline:

- ~50% `common` (the world's recorded history)
- ~25% `regional`
- ~15% `cloistered`
- ~8% `secret`
- ~2% `lost`

Adjust based on world tone: a high-paranoia world skews more
cloistered + secret; a public-facing world stays mostly common.

## `rumoured_as` writing notes

A great rumour:

- Is **shorter** than the true summary (rumours simplify).
- **Reverses or inverts** a key fact (the loser was the winner; the
  willing party was coerced).
- **Names someone the truth doesn't** (rumours need protagonists).
- Is something a character could plausibly believe in a tavern.

# Output

Strict JSON, no preamble, no commentary. Emit a `visibilities` list
that maps event ids to their visibility blocks. The CLI will merge
these back into the event records.

```json
{
  "visibilities": [
    {
      "id": "evt_0027",
      "tier": "common",
      "known_to": [],
      "rumoured_as": "",
      "true_facts_hidden": []
    },
    {
      "id": "evt_0042",
      "tier": "cloistered",
      "known_to": ["asha", "veska", "harbour-guild"],
      "rumoured_as": "The Concord was signed in good faith and ended the blockade by mutual relief.",
      "true_facts_hidden": ["Veska signed under duress.", "A hostage was held during the negotiation."]
    }
  ]
}
```

EVERY event id from every prior pass must appear exactly once in
`visibilities`. First byte of output: `{`. Last byte: `}`. No
markdown fences.
