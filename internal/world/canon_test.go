package world

import (
	"strings"
	"testing"
)

func TestSelectRelevantCanon_SmallPassesThrough(t *testing.T) {
	canon := "## People\n- Asha — the cartographer.\n"
	got := SelectRelevantCanon(canon, "anything", nil, DefaultCanonBudget)
	if got != canon {
		t.Errorf("small canon should pass through verbatim.\n got: %q\nwant: %q", got, canon)
	}
}

func buildBigCanon() string {
	var b strings.Builder
	b.WriteString("## From installment 1\n\n## People\n")
	b.WriteString("- Asha — the cartographer who maps the drowned coast.\n")
	b.WriteString("- Veska — a smuggler who owes the harbour guild a debt.\n")
	b.WriteString("## Rules\n")
	b.WriteString("- Salt-bound oaths break the binder, not the bound.\n")
	b.WriteString("## Places\n")
	b.WriteString("- Vahn's Reach — a port the series returns to.\n")
	// Padding entries to push past the budget; irrelevant to the brief.
	for i := 0; i < 400; i++ {
		b.WriteString("- A minor fact about pottery glaze and kiln temperatures and clay sourcing.\n")
	}
	return b.String()
}

func TestSelectRelevantCanon_FiltersLarge(t *testing.T) {
	canon := buildBigCanon()
	if len(canon) <= 2000 {
		t.Fatalf("test fixture too small: %d", len(canon))
	}
	brief := "This installment follows Asha to Vahn's Reach to settle a cartography dispute."
	got := SelectRelevantCanon(canon, brief, []string{"Asha"}, 2000)

	if len(got) >= len(canon) {
		t.Errorf("filtered canon (%d) should be smaller than full (%d)", len(got), len(canon))
	}
	// Always-keep: world rules.
	if !strings.Contains(got, "Salt-bound oaths") {
		t.Errorf("rules must always be kept; got:\n%s", got)
	}
	// Actor named in the brief + present in canon must be kept.
	if !strings.Contains(got, "Asha") {
		t.Errorf("on-stage actor Asha must be kept; got:\n%s", got)
	}
	// Brief-relevant place should be kept (token overlap on "vahn"/"reach").
	if !strings.Contains(got, "Vahn's Reach") {
		t.Errorf("brief-relevant place should be kept; got:\n%s", got)
	}
	// The flood of irrelevant pottery facts should be largely dropped.
	if strings.Count(got, "pottery glaze") > 20 {
		t.Errorf("expected most irrelevant facts dropped; kept %d", strings.Count(got, "pottery glaze"))
	}
}

func TestSelectRelevantCanon_PreservesHeaders(t *testing.T) {
	canon := buildBigCanon()
	got := SelectRelevantCanon(canon, "Asha cartography", []string{"Asha"}, 2000)
	if !strings.Contains(got, "## People") {
		t.Errorf("category header should be preserved for kept entries; got:\n%s", got)
	}
	if !strings.Contains(got, "## Rules") {
		t.Errorf("rules header should be preserved; got:\n%s", got)
	}
}

func TestParseCanon_Structure(t *testing.T) {
	canon := "## From installment 1\n\n## People\n- Asha — a.\n- Veska — b.\n## Places\n- Reach — c.\n"
	entries, _ := parseCanon(canon)
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3: %+v", len(entries), entries)
	}
	if !strings.Contains(entries[0].category, "People") {
		t.Errorf("entry 0 category = %q, want People", entries[0].category)
	}
	if !strings.Contains(entries[2].category, "Places") {
		t.Errorf("entry 2 category = %q, want Places", entries[2].category)
	}
}
