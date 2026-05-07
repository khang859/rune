package providers

import (
	"testing"

	"github.com/khang859/rune/internal/config"
)

func TestPDFInputSupport(t *testing.T) {
	cases := []struct {
		provider string
		model    string
		want     DocumentSupport
	}{
		{Codex, "gpt-5", DocumentSupported},
		{Codex, "custom-model", DocumentUnknown},
		{Groq, "meta-llama/llama-4-scout-17b-16e-instruct", DocumentUnsupported},
		{Ollama, "qwen3-vl:8b", DocumentUnsupported},
		{Runpod, "openai/gpt-oss-120b", DocumentUnsupported},
	}
	for _, tc := range cases {
		if got := PDFInputSupport(tc.provider, tc.model); got != tc.want {
			t.Fatalf("%s/%s PDF support = %s, want %s", tc.provider, tc.model, got, tc.want)
		}
	}
}

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

func TestToolUseSupport(t *testing.T) {
	cases := []struct {
		provider string
		model    string
		want     ToolSupport
	}{
		{Ollama, "gemma3", ToolUnsupported},
		{Ollama, "gemma2:9b", ToolUnsupported},
		{Ollama, "gemma-3-27b", ToolUnsupported},
		{Ollama, "llama3.2", ToolUnknown},
		{Ollama, "qwen3:8b", ToolUnknown},
		{Codex, "gpt-5.5", ToolUnknown},
		{Groq, "llama-3.3-70b-versatile", ToolUnknown},
		{Runpod, "openai/gpt-oss-120b", ToolUnknown},
	}
	for _, tc := range cases {
		if got := ToolUseSupport(tc.provider, tc.model); got != tc.want {
			t.Fatalf("%s/%s tool support = %s, want %s", tc.provider, tc.model, got, tc.want)
		}
	}
}

func TestToolUseSupportWithSettings(t *testing.T) {
	settings := config.Settings{ModelCapabilities: map[string]config.ModelCapabilities{
		"ollama:gemma3": {Tools: "on"},
		"qwen3:8b":      {Tools: "off"},
	}}
	if got := ToolUseSupportWithSettings(Ollama, "gemma3", settings); got != ToolSupported {
		t.Fatalf("gemma3 override = %s, want supported", got)
	}
	if got := ToolUseSupportWithSettings(Ollama, "qwen3:8b", settings); got != ToolUnsupported {
		t.Fatalf("qwen3 fallback override = %s, want unsupported", got)
	}
	if got := ToolUseSupportWithSettings(Ollama, "llama3.2", settings); got != ToolUnknown {
		t.Fatalf("llama3.2 default = %s, want unknown", got)
	}
}

func TestImageInputSupport_Runpod(t *testing.T) {
	cases := []struct {
		model string
		want  ImageSupport
	}{
		{"openai/gpt-oss-120b", ImageUnsupported},
		{"Qwen/Qwen3-32B-AWQ", ImageUnsupported},
		{"custom-vision-model", ImageUnknown},
	}
	for _, tc := range cases {
		if got := ImageInputSupport(Runpod, tc.model); got != tc.want {
			t.Fatalf("%s support = %s, want %s", tc.model, got, tc.want)
		}
	}
}
