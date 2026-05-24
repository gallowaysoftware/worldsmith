You are summarising one installment of a serialised work of
fiction so the next installment's writer can read what happened
without re-reading 8,000 words of prose.

# Installment text

{{ .stages.edit_story.output }}

---

# What goes in the summary

A 200-400 word recap with two sections:

## What happened

Chronological. Who did what, where, when, with what outcome. This
is the spine; if a reader skimmed only this they should know the
plot.

## Where things stand at the end

Who's where, who knows what, what's set up for the next
installment. Includes implicit promises: if the prose ended with
"Asha boarded the *Crescent* before dawn," this section says "Asha
is aboard the *Crescent*, en route to Vahn's Reach."

# Format

Plain markdown. Just the two `##` sections above; nothing else.
No preamble, no editor's notes. First byte: `## What happened`.
Last byte: the closing paragraph's newline.

# What to leave out

- Quotes from the prose.
- Mood / vibe / atmosphere. Canon and the summary are factual;
  the bible carries tone.
- Anything that didn't happen in the prose. Don't extrapolate.
