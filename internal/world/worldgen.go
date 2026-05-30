package world

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// WorldSeed is the structured world spec emitted by the `worldgen` pipeline's
// seed_world stage. The CLI renders it into the standard worldsmith files
// (world.md + characters.json) so every other command operates on the result.
type WorldSeed struct {
	Name        string          `json:"name"`
	Logline     string          `json:"logline"`
	Setting     string          `json:"setting"`
	History     string          `json:"history"`
	Tone        string          `json:"tone"`
	VisualStyle string          `json:"visual_style"`
	Factions    []string        `json:"factions"`
	Rules       []string        `json:"rules"`
	Characters  []SeedCharacter `json:"characters"`
	Locations   []SeedLocation  `json:"locations"`
}

type SeedCharacter struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Look        string `json:"look"`
	VoiceID     string `json:"voice_id"`
	Personality string `json:"personality"`
}

type SeedLocation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Look        string `json:"look"`
}

// ParseWorldSeed decodes the seed_world JSON.
func ParseWorldSeed(b []byte) (WorldSeed, error) {
	var s WorldSeed
	if err := json.Unmarshal(b, &s); err != nil {
		return WorldSeed{}, fmt.Errorf("parse world seed: %w", err)
	}
	return s, nil
}

// Validate rejects a thin / unusable world so the mill can regenerate rather
// than persist a dud.
func (s WorldSeed) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("world has no name")
	}
	if len(s.Characters) < 3 {
		return fmt.Errorf("world %q has %d characters, want >= 3", s.Name, len(s.Characters))
	}
	if len(s.Locations) < 2 {
		return fmt.Errorf("world %q has %d locations, want >= 2", s.Name, len(s.Locations))
	}
	for i, c := range s.Characters {
		if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Look) == "" {
			return fmt.Errorf("character %d missing name or look", i)
		}
	}
	return nil
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

// Slug derives a filesystem-safe slug from the world name.
func (s WorldSeed) Slug() string {
	out := slugRE.ReplaceAllString(strings.ToLower(s.Name), "-")
	out = strings.Trim(out, "-")
	if out == "" {
		out = "world"
	}
	return out
}

// NarratorVoice is the default Kokoro voice and the fallback when a
// character's assigned voice_id isn't a known voice.
const NarratorVoice = "am_fenrir"

// validVoices is the set of Kokoro voice ids the seed_world prompt offers.
// The LLM occasionally typos one (e.g. "am_fenfir"); an unknown voice makes
// Kokoro 400 and fails the whole scene, so we normalize to NarratorVoice.
var validVoices = map[string]bool{
	"am_fenrir": true, "am_michael": true, "am_puck": true,
	"am_adam": true, "am_eric": true,
	"af_bella": true, "af_nicole": true, "bf_emma": true,
}

// NormalizeVoice returns v if it's a known voice, else NarratorVoice.
func NormalizeVoice(v string) string {
	if validVoices[strings.TrimSpace(v)] {
		return strings.TrimSpace(v)
	}
	return NarratorVoice
}

// charactersForFile is the on-disk characters.json shape (matches what the
// scene_outline prompt reads back: look + voice_id drive image gen + casting).
type charactersForFile struct {
	Characters []SeedCharacter `json:"characters"`
}

// WriteWorldFromSeed renders the seed into world.md + characters.json under
// the layout, plus a verbatim world_seed.json for reference. Does not clobber
// an existing world.md (so a hand-edited world survives a re-run).
func WriteWorldFromSeed(l Layout, s WorldSeed, seedJSON []byte) error {
	if err := os.WriteFile(l.worldSeedFile(), seedJSON, 0o644); err != nil {
		return fmt.Errorf("write world_seed.json: %w", err)
	}
	if _, err := os.Stat(l.WorldFile()); err != nil && os.IsNotExist(err) {
		if err := os.WriteFile(l.WorldFile(), []byte(s.renderWorldMarkdown()), 0o644); err != nil {
			return fmt.Errorf("write world.md: %w", err)
		}
	}
	if _, err := os.Stat(l.CharactersFile()); err != nil && os.IsNotExist(err) {
		// Normalize voice_ids to known Kokoro voices so a hallucinated/typo'd
		// voice can't 400 the TTS stage and fail the whole scene later.
		for i := range s.Characters {
			s.Characters[i].VoiceID = NormalizeVoice(s.Characters[i].VoiceID)
		}
		doc := charactersForFile{Characters: s.Characters}
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal characters: %w", err)
		}
		if err := os.WriteFile(l.CharactersFile(), b, 0o644); err != nil {
			return fmt.Errorf("write characters.json: %w", err)
		}
	}
	// Seed an empty canon so the scene pipeline's optional canon read is happy.
	if _, err := EnsureCanonFile(l); err != nil {
		return err
	}
	return nil
}

func (l Layout) worldSeedFile() string { return l.Root + "/world_seed.json" }

func (s WorldSeed) renderWorldMarkdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Name)
	if s.Logline != "" {
		fmt.Fprintf(&b, "_%s_\n\n", s.Logline)
	}
	fmt.Fprintf(&b, "## Setting\n\n%s\n\n", s.Setting)
	if s.History != "" {
		fmt.Fprintf(&b, "## History\n\n%s\n\n", s.History)
	}
	if s.Tone != "" {
		fmt.Fprintf(&b, "## Tone\n\n%s\n\n", s.Tone)
	}
	if s.VisualStyle != "" {
		fmt.Fprintf(&b, "## Visual style\n\n%s\n\n", s.VisualStyle)
	}
	if len(s.Factions) > 0 {
		b.WriteString("## Factions / forces\n\n")
		for _, f := range s.Factions {
			fmt.Fprintf(&b, "- %s\n", f)
		}
		b.WriteString("\n")
	}
	if len(s.Rules) > 0 {
		b.WriteString("## Rules\n\n")
		for _, r := range s.Rules {
			fmt.Fprintf(&b, "- %s\n", r)
		}
		b.WriteString("\n")
	}
	if len(s.Locations) > 0 {
		b.WriteString("## Locations\n\n")
		for _, loc := range s.Locations {
			fmt.Fprintf(&b, "### %s\n\n%s\n\nLook: %s\n\n", loc.Name, loc.Description, loc.Look)
		}
	}
	return b.String()
}
