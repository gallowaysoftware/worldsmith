package world

import "testing"

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
