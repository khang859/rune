package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/khang859/rune/internal/profile"
	"github.com/khang859/rune/internal/skill"
)

// loadProfile resolves a named worker profile from the global and project-local
// profile directories. It returns nil when name is empty, and an error when a
// named profile cannot be found.
func loadProfile(name, cwd, home string) (*profile.Profile, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil
	}
	p, err := (&profile.Loader{Roots: []string{
		filepath.Join(home, ".rune", "profiles"),
		filepath.Join(cwd, ".rune", "profiles"),
	}}).Resolve(name)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// profileModel applies the profile model precedence: an explicit --model flag
// always wins, otherwise the profile's model is used (empty means "leave to the
// normal provider resolution").
func profileModel(modelOverride string, p *profile.Profile) string {
	if strings.TrimSpace(modelOverride) != "" {
		return modelOverride
	}
	if p != nil {
		return p.Model
	}
	return ""
}

// prependProfile prepends a profile's body to system. When includeSkillBodies
// is set, the bodies of the profile's selected skills are injected too (used by
// non-interactive --prompt mode, which has no way to arm skills at runtime).
// Skills named by the profile but not found on disk are reported on w. The
// matched skills are returned so callers can reuse the selection.
func prependProfile(system string, p *profile.Profile, skills []skill.Skill, includeSkillBodies bool, w io.Writer) (string, []skill.Skill) {
	if p == nil {
		return system, skills
	}
	matched, missing := skill.Filter(skills, p.Skills)
	for _, name := range missing {
		fmt.Fprintf(w, "(profile %q: skill %q not found)\n", p.Name, name)
	}
	var b strings.Builder
	if p.Instructions != "" {
		b.WriteString(p.Instructions)
		b.WriteString("\n\n")
	}
	if includeSkillBodies {
		for _, s := range matched {
			b.WriteString(s.Body)
			b.WriteString("\n\n")
		}
	}
	b.WriteString(system)
	return b.String(), matched
}
