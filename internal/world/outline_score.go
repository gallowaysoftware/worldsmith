package world

import (
	"encoding/json"
	"math"
	"strings"
)

// Outline scoring.
//
// Candidate rerank generates several scene plans and keeps the best
// one. We score plans structurally rather than with another LLM pass:
// the DOC result is that better-structured outlines produce better
// prose, and structure — budget distribution, turning-point placement,
// completeness of the per-scene fields, presence of canon hooks — is
// exactly what code can measure reliably. (An LLM judge could be
// layered on top later; this is the deterministic, testable floor.)

type outlineScene struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Setting        string `json:"setting"`
	Goal           string `json:"goal"`
	Conflict       string `json:"conflict"`
	Turn           string `json:"turn"`
	EmotionalShift string `json:"emotional_shift"`
	Tension        struct {
		Uncertainty string `json:"uncertainty"`
		Hope        string `json:"hope"`
		Fear        string `json:"fear"`
		Withheld    string `json:"withheld"`
	} `json:"tension"`
	CanonHooks []string `json:"canon_hooks"`
	WordBudget int      `json:"word_budget"`
}

type outlineDoc struct {
	InstallmentTargetWords int            `json:"installment_target_words"`
	TurningPointScene      string         `json:"turning_point_scene"`
	Scenes                 []outlineScene `json:"scenes"`
}

// OutlineScore is the 0..100 structural quality of a scene plan, with
// the component breakdown so a caller can log why one candidate won.
type OutlineScore struct {
	Total        float64 `json:"total"`
	BudgetFit    float64 `json:"budget_fit"`    // sum of budgets vs target
	Completeness float64 `json:"completeness"`  // per-scene fields filled
	TurningPoint float64 `json:"turning_point"` // pivot in the back half
	SceneCount   float64 `json:"scene_count"`   // 3..8 sweet spot
	CanonHooks   float64 `json:"canon_hooks"`   // scenes carrying a hook
	Valid        bool    `json:"valid"`         // parsed + has scenes
}

// ScoreOutline parses an outline.json candidate and returns its
// structural score. An unparseable or empty plan scores 0 with
// Valid=false so the caller never picks it over a real candidate.
func ScoreOutline(outlineJSON string) OutlineScore {
	var doc outlineDoc
	if err := json.Unmarshal([]byte(outlineJSON), &doc); err != nil || len(doc.Scenes) == 0 {
		return OutlineScore{}
	}
	n := len(doc.Scenes)
	s := OutlineScore{Valid: true}

	// Budget fit (25): how close the sum of scene budgets lands to the
	// declared target. Full marks within 10%, decaying to 0 by 50% off.
	sum := 0
	for _, sc := range doc.Scenes {
		sum += sc.WordBudget
	}
	target := doc.InstallmentTargetWords
	if target <= 0 {
		target = 7500
	}
	off := math.Abs(float64(sum-target)) / float64(target)
	switch {
	case off <= 0.10:
		s.BudgetFit = 25
	case off >= 0.50:
		s.BudgetFit = 0
	default:
		s.BudgetFit = 25 * (1 - (off-0.10)/0.40)
	}

	// Completeness (35): fraction of the per-scene craft fields that
	// are filled, averaged across scenes. The tension block counts as
	// its four sub-fields.
	const fieldsPerScene = 8 // setting, goal, conflict, turn, emo, unc, hope, fear
	filled := 0
	for _, sc := range doc.Scenes {
		for _, f := range []string{sc.Setting, sc.Goal, sc.Conflict, sc.Turn, sc.EmotionalShift,
			sc.Tension.Uncertainty, sc.Tension.Hope, sc.Tension.Fear} {
			if strings.TrimSpace(f) != "" {
				filled++
			}
		}
	}
	s.Completeness = 35 * float64(filled) / float64(n*fieldsPerScene)

	// Turning point (20): the declared pivot scene sits in the back
	// half of the plan (LLMs default to resolving too early).
	if idx := sceneIndex(doc.Scenes, doc.TurningPointScene); idx >= 0 {
		if idx >= n/2 {
			s.TurningPoint = 20
		} else {
			s.TurningPoint = 8 // named but too early — partial credit
		}
	}

	// Scene count (10): 3..8 scenes is the workable range for a
	// single installment; outside it, taper.
	switch {
	case n >= 3 && n <= 8:
		s.SceneCount = 10
	case n == 2 || n == 9 || n == 10:
		s.SceneCount = 5
	default:
		s.SceneCount = 0
	}

	// Canon hooks (10): fraction of scenes carrying at least one hook.
	withHooks := 0
	for _, sc := range doc.Scenes {
		if len(sc.CanonHooks) > 0 {
			withHooks++
		}
	}
	s.CanonHooks = 10 * float64(withHooks) / float64(n)

	s.Total = s.BudgetFit + s.Completeness + s.TurningPoint + s.SceneCount + s.CanonHooks
	return s
}

func sceneIndex(scenes []outlineScene, id string) int {
	id = strings.TrimSpace(id)
	if id == "" {
		return -1
	}
	for i, sc := range scenes {
		if sc.ID == id {
			return i
		}
	}
	return -1
}
