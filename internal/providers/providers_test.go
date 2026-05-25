package providers

import "testing"

func TestOpenRouterRegistry(t *testing.T) {
	if got := Normalize(" openrouter "); got != OpenRouter {
		t.Fatalf("Normalize(openrouter) = %q", got)
	}
	if got := DefaultModel(OpenRouter); got != DefaultOpenRouterModel {
		t.Fatalf("DefaultModel(OpenRouter) = %q", got)
	}
	if !IsKnownModel(OpenRouter, DefaultOpenRouterModel) {
		t.Fatalf("OpenRouter models should include default %q", DefaultOpenRouterModel)
	}
}
