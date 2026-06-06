package world

import (
	"strings"
	"testing"
)

func TestAnalyzeProse_Basic(t *testing.T) {
	text := "The harbour was quiet. She walked to the water. The bell rang once."
	m := AnalyzeProse(text)
	if m.Words == 0 {
		t.Fatalf("expected non-zero word count")
	}
	if m.Sentences != 3 {
		t.Errorf("sentences = %d, want 3", m.Sentences)
	}
	if m.SlopTotal != 0 {
		t.Errorf("slop on clean text = %d, want 0 (%v)", m.SlopTotal, m.SlopHits)
	}
	if m.NotXButY != 0 {
		t.Errorf("not-x-but-y on clean text = %d, want 0", m.NotXButY)
	}
}

func TestAnalyzeProse_SlopDetected(t *testing.T) {
	text := "The light shimmered, an ethereal symphony, a palpable thrum in the air. " +
		"The pulse of the city was a tapestry."
	m := AnalyzeProse(text)
	for _, term := range []string{"shimmered", "ethereal", "symphony", "palpable", "thrum", "pulse", "tapestry"} {
		if m.SlopHits[term] == 0 {
			t.Errorf("expected slop term %q to be detected; hits=%v", term, m.SlopHits)
		}
	}
	if m.SlopTotal < 7 {
		t.Errorf("slop total = %d, want >= 7", m.SlopTotal)
	}
	if m.SlopPer1000 <= 0 {
		t.Errorf("slop per 1000 should be positive, got %f", m.SlopPer1000)
	}
}

func TestAnalyzeProse_SlopWholeWordOnly(t *testing.T) {
	// "impulse" contains "pulse" but must not trip the single-word
	// matcher (which keys off tokenization, not substring).
	m := AnalyzeProse("He acted on impulse alone.")
	if m.SlopHits["pulse"] != 0 {
		t.Errorf("'impulse' wrongly matched slop term 'pulse'")
	}
}

func TestAnalyzeProse_NotXButY(t *testing.T) {
	text := "It wasn't anger, but something colder. She did not run, but she wanted to. " +
		"The room was warm and bright."
	m := AnalyzeProse(text)
	if m.NotXButY < 2 {
		t.Errorf("not-x-but-y = %d, want >= 2", m.NotXButY)
	}
}

func TestAnalyzeProse_RepeatedOpeners(t *testing.T) {
	text := "He thought of her. He thought of the sea. He thought of nothing. He thought of home."
	m := AnalyzeProse(text)
	var found bool
	for _, o := range m.RepeatedOpeners {
		if o.Phrase == "he thought" && o.Count >= 4 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected repeated opener 'he thought' (>=4); got %v", m.RepeatedOpeners)
	}
}

func TestAnalyzeProse_RepeatedTrigrams(t *testing.T) {
	text := "out of the dark and out of the dark and out of the dark"
	m := AnalyzeProse(text)
	var found bool
	for _, tg := range m.RepeatedTrigrams {
		if tg.Phrase == "out of the" && tg.Count >= 3 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected repeated trigram 'out of the' (>=3); got %v", m.RepeatedTrigrams)
	}
}

func TestAnalyzeProse_Empty(t *testing.T) {
	m := AnalyzeProse("")
	if m.Words != 0 || m.SlopTotal != 0 || m.SlopPer1000 != 0 {
		t.Errorf("empty text should yield zero metrics, got %+v", m)
	}
}

func TestOffendingSentences(t *testing.T) {
	text := "It was a long and quiet morning aboard the orbital station. " +
		"It was colder there than the flight logs had predicted. " +
		"It was the third alarm that finally woke the sleeping crew. " + // 3rd "it was" -> flagged
		"It was nothing the thick manual had ever prepared them for. " + // 4th "it was" -> flagged
		"The old reactor pulsed against its worn magnetic bottle. " + // slop "pulsed"
		"The verdict was not mercy but a slower kind of cruelty." // not-X-but-Y

	spans := OffendingSentences(text, 40)
	if len(spans) != 4 {
		t.Fatalf("flagged %d sentences, want 4: %+v", len(spans), spans)
	}
	// Every span must be a verbatim substring so it can be spliced back.
	for _, s := range spans {
		if !strings.Contains(text, s.Span) {
			t.Errorf("span not found verbatim in text: %q", s.Span)
		}
	}
	// The first two "It was" sentences are kept (variety not over-corrected); only
	// the excess are flagged.
	if strings.Contains(spans[0].Span, "long and quiet morning") {
		t.Errorf("first repeated opener should be kept, not flagged")
	}
	joined := ""
	for _, s := range spans {
		joined += s.Reason + "\n"
	}
	for _, want := range []string{"repeated opener", "slop", "not-X-but-Y"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing reason %q in:\n%s", want, joined)
		}
	}

	// Clean prose flags nothing.
	if got := OffendingSentences("The harbour was quiet. She walked to the cold water. A bell rang once.", 40); got != nil {
		t.Errorf("clean prose flagged %d sentences, want 0", len(got))
	}
}
