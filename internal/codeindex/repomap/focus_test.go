package repomap

import (
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestExtractMentionedIdents(t *testing.T) {
	msgs := []ai.Message{
		{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "Please fix parseConfig in loop.go"}}},
		{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "Looking at HandleTool now"}}},
	}
	symbols := map[string]bool{
		"parseConfig": true,
		"HandleTool":  true,
		"unused":      true,
	}
	got := ExtractMentionedIdents(msgs, symbols)
	if !got["parseConfig"] {
		t.Errorf("missing parseConfig: %v", got)
	}
	if !got["HandleTool"] {
		t.Errorf("missing HandleTool: %v", got)
	}
	if got["unused"] {
		t.Errorf("should not include symbols absent from chat: %v", got)
	}
}

func TestExtractMentionedIdentsFiltersStopwords(t *testing.T) {
	msgs := []ai.Message{
		{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "func error return string"}}},
	}
	symbols := map[string]bool{"func": true, "error": true, "return": true, "string": true}
	got := ExtractMentionedIdents(msgs, symbols)
	if len(got) != 0 {
		t.Errorf("stopwords should be filtered, got %v", got)
	}
}

func TestExtractMentionedIdentsRequiresMinLength(t *testing.T) {
	msgs := []ai.Message{
		{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "use x or ab; try parseConfig"}}},
	}
	symbols := map[string]bool{"x": true, "ab": true, "parseConfig": true}
	got := ExtractMentionedIdents(msgs, symbols)
	if got["x"] || got["ab"] {
		t.Errorf("short idents should be filtered: %v", got)
	}
	if !got["parseConfig"] {
		t.Errorf("parseConfig missing: %v", got)
	}
}
