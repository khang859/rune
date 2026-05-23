package repomap

import (
	"regexp"

	"github.com/khang859/rune/internal/ai"
)

// Focus is the per-turn signal fed into ranking. InFocusFiles biases the
// PageRank personalization vector; MentionedIdents boosts edge weights and
// per-file symbol selection.
type Focus struct {
	InFocusFiles    []string
	MentionedIdents map[string]bool
}

var identRegexp = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

// goKeywords and friends are stopwords too common to ever be useful as
// mentioned idents even if the index happens to contain a symbol with the
// same name (rare but possible for fields like "type").
var stopwords = map[string]bool{
	"func": true, "return": true, "error": true, "string": true, "int": true,
	"bool": true, "true": true, "false": true, "nil": true, "type": true,
	"struct": true, "interface": true, "package": true, "import": true,
	"var": true, "const": true, "for": true, "range": true, "switch": true,
	"case": true, "default": true, "break": true, "continue": true,
	"if": true, "else": true, "this": true, "that": true, "the": true,
	"and": true, "you": true, "with": true, "from": true, "into": true,
	"will": true, "should": true, "would": true, "could": true, "have": true,
	"been": true, "they": true, "them": true, "what": true, "when": true,
	"where": true, "which": true, "while": true, "your": true, "but": true,
}

// ExtractMentionedIdents scans message text for identifier-shaped tokens,
// filters stopwords, and keeps only those present in symbolNames.
func ExtractMentionedIdents(messages []ai.Message, symbolNames map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, m := range messages {
		for _, c := range m.Content {
			text := textOf(c)
			if text == "" {
				continue
			}
			for _, match := range identRegexp.FindAllString(text, -1) {
				if stopwords[match] {
					continue
				}
				if symbolNames[match] {
					out[match] = true
				}
			}
		}
	}
	return out
}

func textOf(c ai.ContentBlock) string {
	switch v := c.(type) {
	case ai.TextBlock:
		return v.Text
	case ai.ToolResultBlock:
		return v.Output
	default:
		return ""
	}
}
