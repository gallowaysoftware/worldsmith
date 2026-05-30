package world

import "testing"

const goodOutline = `{
  "installment_target_words": 6000,
  "turning_point_scene": "scene_3",
  "scenes": [
    {"id":"scene_1","title":"a","setting":"s","goal":"g","conflict":"c","turn":"t","emotional_shift":"x→y","tension":{"uncertainty":"u","hope":"h","fear":"f","withheld":"w"},"canon_hooks":["e1"],"word_budget":1500},
    {"id":"scene_2","title":"b","setting":"s","goal":"g","conflict":"c","turn":"t","emotional_shift":"x→y","tension":{"uncertainty":"u","hope":"h","fear":"f","withheld":"w"},"canon_hooks":["e2"],"word_budget":1500},
    {"id":"scene_3","title":"c","setting":"s","goal":"g","conflict":"c","turn":"t","emotional_shift":"x→y","tension":{"uncertainty":"u","hope":"h","fear":"f","withheld":"w"},"canon_hooks":["e3"],"word_budget":1500},
    {"id":"scene_4","title":"d","setting":"s","goal":"g","conflict":"c","turn":"t","emotional_shift":"x→y","tension":{"uncertainty":"u","hope":"h","fear":"f","withheld":"w"},"canon_hooks":["e4"],"word_budget":1500}
  ]
}`

// Poor: budgets way under target, no tension, turning point in scene 1, no hooks.
const poorOutline = `{
  "installment_target_words": 6000,
  "turning_point_scene": "scene_1",
  "scenes": [
    {"id":"scene_1","title":"a","goal":"g","word_budget":300},
    {"id":"scene_2","title":"b","goal":"g","word_budget":300}
  ]
}`

func TestScoreOutline_GoodBeatsPoor(t *testing.T) {
	good := ScoreOutline(goodOutline)
	poor := ScoreOutline(poorOutline)
	if !good.Valid || !poor.Valid {
		t.Fatalf("both should be valid; good=%v poor=%v", good.Valid, poor.Valid)
	}
	if good.Total <= poor.Total {
		t.Errorf("good (%.1f) should outscore poor (%.1f)", good.Total, poor.Total)
	}
}

func TestScoreOutline_GoodIsHigh(t *testing.T) {
	good := ScoreOutline(goodOutline)
	if good.BudgetFit != 25 {
		t.Errorf("budget fit = %.1f, want 25 (sum 6000 == target)", good.BudgetFit)
	}
	if good.TurningPoint != 20 {
		t.Errorf("turning point = %.1f, want 20 (scene_3 of 4 is back half)", good.TurningPoint)
	}
	if good.Total < 85 {
		t.Errorf("a complete outline should score high; got %.1f", good.Total)
	}
}

func TestScoreOutline_TurningPointEarlyPartial(t *testing.T) {
	poor := ScoreOutline(poorOutline)
	if poor.TurningPoint != 8 {
		t.Errorf("early turning point should get partial 8; got %.1f", poor.TurningPoint)
	}
}

func TestScoreOutline_Invalid(t *testing.T) {
	for _, bad := range []string{"", "not json", `{"scenes":[]}`, `{}`} {
		s := ScoreOutline(bad)
		if s.Valid || s.Total != 0 {
			t.Errorf("input %q should score 0/invalid; got %+v", bad, s)
		}
	}
}
