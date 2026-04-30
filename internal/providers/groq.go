package providers

// GroqReasoningEffort maps a rune-level effort setting ("none"/"low"/"medium"/
// "high"/"xhigh") to the wire value Groq accepts for `reasoning_effort` on the
// given model. Returns "" when the field should be omitted entirely — either
// because the model rejects `reasoning_effort` (e.g. Llama, DeepSeek) or
// because the user picked "none".
//
// Per Groq docs (console.groq.com/docs/reasoning):
//   - GPT-OSS 20B/120B: accepts "low"/"medium"/"high"
//   - Qwen3 32B: accepts "none"/"default" only
//   - All other models: rejects the field with a 400
func GroqReasoningEffort(model, effort string) string {
	if effort == "" || effort == "none" {
		return ""
	}
	switch model {
	case "openai/gpt-oss-120b", "openai/gpt-oss-20b":
		switch effort {
		case "low", "medium", "high":
			return effort
		case "xhigh":
			return "high"
		}
		return ""
	case "qwen/qwen3-32b":
		return "default"
	}
	return ""
}

// GroqThinkingLevels returns the rune-level effort values to expose in the UI
// for a given Groq model, or nil if the model doesn't support reasoning_effort.
func GroqThinkingLevels(model string) []string {
	switch model {
	case "openai/gpt-oss-120b", "openai/gpt-oss-20b", "qwen/qwen3-32b":
		return []string{"none", "low", "medium", "high"}
	}
	return nil
}
