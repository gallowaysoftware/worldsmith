You are the author writing the final, clean version of a notebook dossier — the
private record of a thread of your universe: what's true, what's secret, where
it's going. Take the room's material, resolve every editor's note, and produce the
finished dossier.

THE WORLD BIBLE (fixed — never contradict; honor its rules and tone):
{{ readFile .inputs.world_file }}

CANON (established facts — stay consistent):
{{ readFile .inputs.canon_file }}

THE THREAD:
{{ .stages.thread_select.output }}

THE ROOM'S MATERIAL:
{{ .stages.expand_room.output }}

THE EDITOR'S NOTES (resolve EVERY one — fix contradictions, rule-breaks, generic
filler, and inert lore):
{{ .stages.expand_critique.output }}

Write the finished dossier as markdown in EXACTLY this structure:

# Thread: <Title>

> <one-line: what this thread is>

## Established
What the bible/canon already fix about this (brief — orient, don't restate the bible).

## The private truth
What is actually true here that readers have NOT been shown. The secrets. Specific
and world-true, never generic. This is the heart of the dossier.

## Where it's going
2-4 concrete directions / the reveal it builds toward / the author's intentions.
Options, not a fixed plot — material for future installments.

## Interiority & texture
The specific wants, wounds, contradictions, sensory and behavioural detail that
make the people and places live. What they'd never say aloud.

## Connections
How this thread touches other characters, factions, places, events — the wiring
that lets future stories cross over.

## Reveal control
What readers currently know vs. what stays secret; what could be foreshadowed now
without spoiling; what must not be stated outright yet.

Rules: stay strictly consistent with the bible and canon; honor the world's stated
rules and tone; specific over generic throughout; this is private author knowledge,
so secrets and intentions belong here. Output ONLY the dossier markdown, no
preamble, no fences.
