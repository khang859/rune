package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/profile"
	"github.com/khang859/rune/internal/skill"
)

func TestProfileModelPrecedence(t *testing.T) {
	p := &profile.Profile{Model: "profile-model"}
	if got := profileModel("flag-model", p); got != "flag-model" {
		t.Errorf("--model should win: got %q", got)
	}
	if got := profileModel("", p); got != "profile-model" {
		t.Errorf("profile model should apply: got %q", got)
	}
	if got := profileModel("", nil); got != "" {
		t.Errorf("no profile, no flag => empty: got %q", got)
	}
}

func TestPrependProfileNil(t *testing.T) {
	skills := []skill.Skill{{Slug: "a", Body: "abody"}}
	sys, got := prependProfile("BASE", nil, skills, true, &bytes.Buffer{})
	if sys != "BASE" {
		t.Errorf("nil profile must not change system: %q", sys)
	}
	if len(got) != 1 {
		t.Errorf("nil profile must pass skills through: %v", got)
	}
}

func TestPrependProfileInjectsBodyAndSkills(t *testing.T) {
	p := &profile.Profile{Name: "researcher", Instructions: "PERSONA", Skills: []string{"web-research", "missing"}}
	skills := []skill.Skill{{Slug: "web-research", Body: "SKILLBODY"}, {Slug: "other", Body: "x"}}
	var warn bytes.Buffer

	sys, matched := prependProfile("BASE", p, skills, true, &warn)

	if !strings.HasPrefix(sys, "PERSONA") {
		t.Errorf("persona must be prepended: %q", sys)
	}
	if !strings.Contains(sys, "SKILLBODY") {
		t.Errorf("selected skill body must be injected: %q", sys)
	}
	if !strings.HasSuffix(sys, "BASE") {
		t.Errorf("base prompt must remain at the end: %q", sys)
	}
	if len(matched) != 1 || matched[0].Slug != "web-research" {
		t.Errorf("matched = %v, want [web-research]", matched)
	}
	if !strings.Contains(warn.String(), "missing") {
		t.Errorf("missing skill must warn: %q", warn.String())
	}
}

func TestPrependProfileInteractiveOmitsSkillBodies(t *testing.T) {
	p := &profile.Profile{Name: "researcher", Instructions: "PERSONA", Skills: []string{"web-research"}}
	skills := []skill.Skill{{Slug: "web-research", Body: "SKILLBODY"}}

	sys, matched := prependProfile("BASE", p, skills, false, &bytes.Buffer{})

	if strings.Contains(sys, "SKILLBODY") {
		t.Errorf("interactive must not inject skill bodies: %q", sys)
	}
	if len(matched) != 1 {
		t.Errorf("interactive must still return filtered skills: %v", matched)
	}
}

func TestLoadProfile(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	dir := filepath.Join(home, ".rune", "profiles")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "researcher.md"), []byte("---\nname: researcher\nmodel: gpt-4\n---\nbody"), 0o600); err != nil {
		t.Fatal(err)
	}

	if p, err := loadProfile("", cwd, home); err != nil || p != nil {
		t.Errorf("empty name => (nil, nil): got (%v, %v)", p, err)
	}
	p, err := loadProfile("researcher", cwd, home)
	if err != nil || p == nil || p.Model != "gpt-4" {
		t.Errorf("known profile: got (%v, %v)", p, err)
	}
	if _, err := loadProfile("ghost", cwd, home); err == nil {
		t.Error("unknown profile must error")
	}
}
