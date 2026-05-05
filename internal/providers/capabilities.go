package providers

import "strings"

type ImageSupport string

type DocumentSupport string

type ToolSupport string

const (
	ImageUnsupported ImageSupport = "unsupported"
	ImageSupported   ImageSupport = "supported"
	ImageUnknown     ImageSupport = "unknown"

	DocumentUnsupported DocumentSupport = "unsupported"
	DocumentSupported   DocumentSupport = "supported"
	DocumentUnknown     DocumentSupport = "unknown"

	ToolUnsupported ToolSupport = "unsupported"
	ToolSupported   ToolSupport = "supported"
	ToolUnknown     ToolSupport = "unknown"
)

func PDFInputSupport(provider, model string) DocumentSupport {
	model = strings.ToLower(strings.TrimSpace(model))
	switch Normalize(provider) {
	case Groq, Ollama, Runpod:
		return DocumentUnsupported
	default:
		if isKnownCodexModel(model) || looksLikeOpenAIFileModel(model) {
			return DocumentSupported
		}
		return DocumentUnknown
	}
}

func ImageInputSupport(provider, model string) ImageSupport {
	model = strings.ToLower(strings.TrimSpace(model))
	switch Normalize(provider) {
	case Groq:
		switch model {
		case "meta-llama/llama-4-maverick-17b-128e-instruct",
			"meta-llama/llama-4-scout-17b-16e-instruct":
			return ImageSupported
		}
		if IsKnownModel(Groq, model) {
			return ImageUnsupported
		}
		return ImageUnknown
	case Ollama:
		if looksLikeOllamaVisionModel(model) {
			return ImageSupported
		}
		if IsKnownModel(Ollama, model) {
			return ImageUnsupported
		}
		return ImageUnknown
	case Runpod:
		if IsKnownModel(Runpod, model) {
			return ImageUnsupported
		}
		return ImageUnknown
	default:
		if isKnownCodexModel(model) || looksLikeOpenAIVisionModel(model) {
			return ImageSupported
		}
		return ImageUnknown
	}
}

func isKnownCodexModel(model string) bool {
	for _, m := range CodexModels {
		if strings.EqualFold(m, model) {
			return true
		}
	}
	return false
}

func looksLikeOpenAIFileModel(model string) bool {
	for _, prefix := range []string{
		"gpt-5",
		"gpt-4.1",
		"gpt-4o",
		"o3",
		"o4",
		"chatgpt-4o",
	} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

func looksLikeOpenAIVisionModel(model string) bool {
	for _, prefix := range []string{
		"gpt-5",
		"gpt-4.1",
		"gpt-4o",
		"o3",
		"o4",
		"chatgpt-4o",
	} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	return false
}

// ToolUseSupport reports whether the (provider, model) pair is known to support
// the function/tool-calling protocol used by the agent loop. Returning
// ToolUnsupported tells callers to drop tools from the request entirely;
// chat-only models like Google's Gemma family reject requests that include a
// tools array. ToolUnknown means we should send tools and let the provider
// decide.
func ToolUseSupport(provider, model string) ToolSupport {
	model = strings.ToLower(strings.TrimSpace(model))
	switch Normalize(provider) {
	case Ollama:
		if looksLikeChatOnlyOllamaModel(model) {
			return ToolUnsupported
		}
		return ToolUnknown
	default:
		return ToolUnknown
	}
}

// looksLikeChatOnlyOllamaModel matches Ollama tags for model families that
// don't implement function calling. Keep this list narrow — false positives
// silently strip tools and break the agent.
func looksLikeChatOnlyOllamaModel(model string) bool {
	chatOnlyHints := []string{
		"gemma",
	}
	for _, hint := range chatOnlyHints {
		if strings.Contains(model, hint) {
			return true
		}
	}
	return false
}

func looksLikeOllamaVisionModel(model string) bool {
	visionHints := []string{
		"-vl",
		":vl",
		"vl:",
		"vision",
		"llava",
		"bakllava",
		"moondream",
		"minicpm-v",
		"minicpmv",
		"qwen-vl",
		"qwen2-vl",
		"qwen2.5-vl",
		"qwen3-vl",
		"gemma3",
		"gemma-3",
		"llama3.2-vision",
		"llama-3.2-vision",
	}
	for _, hint := range visionHints {
		if strings.Contains(model, hint) {
			return true
		}
	}
	return false
}
