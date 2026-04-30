package providers

import "strings"

const (
	Codex = "codex"
	Groq  = "groq"

	DefaultCodexModel = "gpt-5.5"
	DefaultGroqModel  = "llama-3.3-70b-versatile"
)

type Info struct {
	ID           string
	Display      string
	DefaultModel string
	Models       []string
}

var CodexModels = []string{
	"gpt-5.5",
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.3-codex",
	"gpt-5.3-codex-spark",
	"gpt-5.2",
	"gpt-5.2-codex",
	"gpt-5.1",
	"gpt-5.1-codex-max",
	"gpt-5.1-codex-mini",
}

var GroqModels = []string{
	"llama-3.3-70b-versatile",
	"openai/gpt-oss-120b",
	"openai/gpt-oss-20b",
	"llama-3.1-8b-instant",
	"meta-llama/llama-4-maverick-17b-128e-instruct",
	"meta-llama/llama-4-scout-17b-16e-instruct",
	"qwen/qwen3-32b",
	"deepseek-r1-distill-llama-70b",
}

func Normalize(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case Groq:
		return Groq
	default:
		return Codex
	}
}

func All() []Info {
	return []Info{
		{ID: Codex, Display: "Codex", DefaultModel: DefaultCodexModel, Models: CodexModels},
		{ID: Groq, Display: "Groq", DefaultModel: DefaultGroqModel, Models: GroqModels},
	}
}

func IDs() []string { return []string{Codex, Groq} }

func Models(provider string) []string {
	if Normalize(provider) == Groq {
		return append([]string(nil), GroqModels...)
	}
	return append([]string(nil), CodexModels...)
}

func DefaultModel(provider string) string {
	if Normalize(provider) == Groq {
		return DefaultGroqModel
	}
	return DefaultCodexModel
}

func IsKnownModel(provider, model string) bool {
	for _, m := range Models(provider) {
		if m == model {
			return true
		}
	}
	return false
}
