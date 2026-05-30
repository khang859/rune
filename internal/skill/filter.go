package skill

import "strings"

// Filter selects skills whose slug matches one of names, preserving the order
// of names. It returns the matched skills and the names that had no matching
// skill loaded.
func Filter(skills []Skill, names []string) (matched []Skill, missing []string) {
	bySlug := make(map[string]Skill, len(skills))
	for _, s := range skills {
		bySlug[s.Slug] = s
	}
	for _, name := range names {
		slug := strings.TrimSpace(name)
		if s, ok := bySlug[slug]; ok {
			matched = append(matched, s)
		} else {
			missing = append(missing, name)
		}
	}
	return matched, missing
}
