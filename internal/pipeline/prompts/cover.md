You are writing the SDXL prompt for the cover art of one
installment of a serialised work of fiction. The cover should be a
*visual anchor* for this specific installment, not a generic
world-cover — a reader recognising the cover should be reminded of
what happened in this particular story.

# World bible (for visual tone)

{{ readFile .inputs.world_file }}

# This installment's brief

{{ readFile .inputs.brief_file }}

# A sample of the editor's prose

{{ .stages.edit_story.output }}

---

# Cover constraints

1. **One concrete subject from this installment.** Pull an image
   the prose actually stated — an object, a place, a tableau —
   that anchors the reader's memory of this story. NOT a face
   (SDXL faces are unreliable and break series cohesion); NOT a
   diagram. A thing or a place.

2. **Visual register inherited from the world bible.** Read the
   bible's `tone` section. If it says "Cormac McCarthy spare,"
   the cover is desaturated and bleak; if it says "Tolkien
   melancholy," lush and twilit; if "Hyperion baroque," dense
   and ornamental. Match the bible's literary register in pixels.

3. **No legible text.** SDXL can't render text reliably. Don't
   ask for words on the image; if text appears in the subject
   (a sign, a book spine), render it as soft blur.

4. **No human faces.** Faces destabilise the cover-to-cover look
   across installments. Hands, silhouettes from behind, figures
   at a distance are fine.

5. **Album-art friendly composition.** Centred subject,
   comfortable margins, looks good cropped square. The user's
   audiobook library renders these at 250-400px.

# Output

Return ONLY the SDXL positive prompt as a single line. No
commentary, no preamble, no negative prompt. Format:

```
<centred subject from this installment>, <one or two sensory details>, <tone-matched aesthetic descriptors from the bible>, centred composition, no text, no faces
```

First byte: a lowercase letter (the start of the subject). No
quotation marks, no JSON wrapper, no explanation.
