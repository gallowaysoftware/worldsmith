package world

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleSeed = `{
  "name": "The Verdant Rust",
  "logline": "Scavengers in a fungal space station.",
  "setting": "A dying orbital station full of spores.",
  "history": "A terraforming experiment went wrong.",
  "tone": "claustrophobic bio-horror",
  "visual_style": "neon cyan and magenta bioluminescence, rusted iron, 8k",
  "factions": ["Spore-Walkers", "Iron Cult"],
  "rules": ["Unfiltered air grows fungus in lungs."],
  "characters": [
    {"name": "Kaelen", "role": "protagonist", "look": "30s, lean, scarred", "voice_id": "am_fenrir", "personality": "wary"},
    {"name": "Jinx", "role": "foil", "look": "20s, shaved head, tattoos", "voice_id": "af_bella", "personality": "greedy"},
    {"name": "Aris", "role": "mentor", "look": "60s, stooped", "voice_id": "am_adam", "personality": "patient"}
  ],
  "locations": [
    {"name": "Hydroponics Hub", "description": "overgrown greenhouse", "look": "glowing mushrooms"},
    {"name": "Core Chamber", "description": "dormant reactor", "look": "amber crystal"}
  ]
}`

func TestParseAndValidateWorldSeed(t *testing.T) {
	s, err := ParseWorldSeed([]byte(sampleSeed))
	if err != nil {
		t.Fatalf("ParseWorldSeed: %v", err)
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if s.Slug() != "the-verdant-rust" {
		t.Errorf("Slug = %q, want the-verdant-rust", s.Slug())
	}
	if len(s.Characters) != 3 || s.Characters[1].VoiceID != "af_bella" {
		t.Errorf("characters parsed wrong: %+v", s.Characters)
	}
}

func TestValidateRejectsThinWorld(t *testing.T) {
	base := func() WorldSeed {
		s, _ := ParseWorldSeed([]byte(sampleSeed))
		return s
	}
	cases := map[string]func(*WorldSeed){
		"no name":        func(s *WorldSeed) { s.Name = "" },
		"few characters": func(s *WorldSeed) { s.Characters = s.Characters[:1] },
		"few locations":  func(s *WorldSeed) { s.Locations = nil },
		"char no look":   func(s *WorldSeed) { s.Characters[0].Look = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := base()
			mutate(&s)
			if err := s.Validate(); err == nil {
				t.Errorf("expected validation error for %s", name)
			}
		})
	}
}

func TestWriteWorldFromSeed(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)
	l, err := Open("test-world")
	if err != nil {
		t.Fatal(err)
	}
	s, _ := ParseWorldSeed([]byte(sampleSeed))
	if err := WriteWorldFromSeed(l, s, []byte(sampleSeed)); err != nil {
		t.Fatalf("WriteWorldFromSeed: %v", err)
	}
	wm, err := os.ReadFile(l.WorldFile())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# The Verdant Rust", "## Setting", "## Visual style", "### Hydroponics Hub", "claustrophobic bio-horror"} {
		if !strings.Contains(string(wm), want) {
			t.Errorf("world.md missing %q", want)
		}
	}
	var doc charactersForFile
	cj, _ := os.ReadFile(l.CharactersFile())
	if err := json.Unmarshal(cj, &doc); err != nil {
		t.Fatalf("characters.json parse: %v", err)
	}
	if len(doc.Characters) != 3 {
		t.Errorf("characters.json has %d, want 3", len(doc.Characters))
	}
	if _, err := os.Stat(filepath.Join(l.Root, "world_seed.json")); err != nil {
		t.Errorf("world_seed.json not written: %v", err)
	}
	// Canon seeded.
	if _, err := os.Stat(l.CanonFile()); err != nil {
		t.Errorf("canon.md not seeded: %v", err)
	}
}
