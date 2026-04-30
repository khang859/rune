package providers

import "strings"

const (
	Codex  = "codex"
	Groq   = "groq"
	Ollama = "ollama"

	DefaultCodexModel  = "gpt-5.5"
	DefaultGroqModel   = "llama-3.3-70b-versatile"
	DefaultOllamaModel = "llama3.2"
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

// OllamaModels is a fallback/suggestion list only. Ollama model names are local
// tags controlled by the user, so callers must accept arbitrary model IDs.
var OllamaModels = []string{
	"llama3.2",
	"qwen3:4b",
	"qwen3:8b",
	"qwen2.5-coder:7b",
	"qwen2.5-coder:14b",
	"deepseek-r1:8b",
	"gpt-oss:20b",
}

func Normalize(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case Groq:
		return Groq
	case Ollama:
		return Ollama
	default:
		return Codex
	}
}

func All() []Info {
	return []Info{
		{ID: Codex, Display: "Codex", DefaultModel: DefaultCodexModel, Models: CodexModels},
		{ID: Groq, Display: "Groq", DefaultModel: DefaultGroqModel, Models: GroqModels},
		{ID: Ollama, Display: "Ollama", DefaultModel: DefaultOllamaModel, Models: OllamaModels},
	}
}

func IDs() []string { return []string{Codex, Groq, Ollama} }

func Models(provider string) []string {
	switch Normalize(provider) {
	case Groq:
		return append([]string(nil), GroqModels...)
	case Ollama:
		return append([]string(nil), OllamaModels...)
	default:
		return append([]string(nil), CodexModels...)
	}
}

func DefaultModel(provider string) string {
	switch Normalize(provider) {
	case Groq:
		return DefaultGroqModel
	case Ollama:
		return DefaultOllamaModel
	default:
		return DefaultCodexModel
	}
}

func IsKnownModel(provider, model string) bool {
	for _, m := range Models(provider) {
		if m == model {
			return true
		}
	}
	return false
}
