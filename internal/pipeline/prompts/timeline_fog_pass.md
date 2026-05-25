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

- **`common`**: appears in widely-distributed chronicles; an
  educated person from any region could plausibly have heard of it,
  even if their version is fuzzy on details. The prose can
  reference it freely. **This is the default tier for major
  recorded history** — foundings of cities, wars, named monarchs,
  plagues, treaties, technological breakthroughs, famines.
  "Common" doesn't mean "every peasant knows it"; it means "it's
  in the world's shared historical record." Set `known_to: []`
  (everyone knows). Set `rumoured_as: ""`.

- **`regional`**: known within `event.region` but **genuinely
  didn't propagate beyond it**. Use for events that mattered
  locally and faded — a local hero who didn't grow famous, a
  guild charter that stayed obscure, a flood whose only record is
  the parish that drowned. If you find yourself reaching for
  `regional` because the event "started in" one region, pause:
  most major historical events START somewhere and propagate.
  Only mark `regional` when propagation genuinely didn't happen.
  Set `known_to: []` (the region's residents know automatically).
  Set `rumoured_as` to whatever vague distortion non-region
  observers might have heard, or `""` if no echo carried.

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

**Most events should be `common`.** The world's recorded history is
the substrate; secrets and forgotten lore are the deviations from
it. A timeline where most events are regional or secret is a
timeline where the world feels small and paranoid for no reason.

Sane targets for a 40-event timeline:

- ~50% `common` (the world's recorded history — the default)
- ~25% `regional` (events whose ripple genuinely didn't propagate)
- ~15% `cloistered` (secret-but-some-actors-know plot fuel)
- ~8% `secret` (true conspiracies; pay off across installments)
- ~2% `lost` (pre-history / forgotten)

**Failure mode to avoid:** classifying everything as `regional`
because each event has a region tag. The region tag says where it
*happened*, not how far its consequences carried. The Plague of
Loss happened in a region — but its consequences reshape the
world's view of medicine, so it's `common`. A particular skirmish
between two villages over a well happened in a region AND stayed
there — that's `regional`.

Adjust based on world tone: a high-paranoia world skews more
cloistered + secret; a public-facing world stays mostly common.
But the defaults above are the starting point, not a ceiling on
`common`.

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
