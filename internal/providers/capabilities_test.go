package providers

import "testing"

func TestImageInputSupport_Groq(t *testing.T) {
	cases := []struct {
		model string
		want  ImageSupport
	}{
		{"meta-llama/llama-4-scout-17b-16e-instruct", ImageSupported},
		{"meta-llama/llama-4-maverick-17b-128e-instruct", ImageSupported},
		{"llama-3.3-70b-versatile", ImageUnsupported},
		{"openai/gpt-oss-120b", ImageUnsupported},
		{"custom-vision-model", ImageUnknown},
	}
	for _, tc := range cases {
		if got := ImageInputSupport(Groq, tc.model); got != tc.want {
			t.Fatalf("%s support = %s, want %s", tc.model, got, tc.want)
		}
	}
}

func TestImageInputSupport_Codex(t *testing.T) {
	for _, model := range []string{"gpt-5.5", "gpt-5.3-codex", "gpt-4o-mini"} {
		if got := ImageInputSupport(Codex, model); got != ImageSupported {
			t.Fatalf("%s support = %s, want %s", model, got, ImageSupported)
		}
	}
	if got := ImageInputSupport(Codex, "custom-model"); got != ImageUnknown {
		t.Fatalf("custom-model support = %s, want %s", got, ImageUnknown)
	}
}

func TestImageInputSupport_Ollama(t *testing.T) {
	cases := []struct {
		model string
		want  ImageSupport
	}{
		{"qwen3-vl:8b", ImageSupported},
		{"gemma3", ImageSupported},
		{"llava:latest", ImageSupported},
		{"llama3.2", ImageUnsupported},
		{"my-local-model", ImageUnknown},
	}
	for _, tc := range cases {
		if got := ImageInputSupport(Ollama, tc.model); got != tc.want {
			t.Fatalf("%s support = %s, want %s", tc.model, got, tc.want)
		}
	}
}
