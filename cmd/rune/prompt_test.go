package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai/oauth"
)

func fakeAccessToken(accountID string) string {
	claim := map[string]any{
		oauth.CodexJWTClaimPath: map[string]any{"chatgpt_account_id": accountID},
	}
	pb, _ := json.Marshal(claim)
	return fmt.Sprintf("h.%s.s", base64.RawURLEncoding.EncodeToString(pb))
}

func TestRunPrompt_HitsCodexAndStreamsText(t *testing.T) {
	sse := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{}}}\n\n"

	codex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("chatgpt-account-id"); got != "acct_xyz" {
			t.Errorf("chatgpt-account-id = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer codex.Close()

	refresh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "AT-NEW",
			"refresh_token": "RT",
			"expires_in":    3600,
		})
	}))
	defer refresh.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_CODEX_ENDPOINT", codex.URL+"/codex/responses")
	t.Setenv("RUNE_OAUTH_TOKEN_URL", refresh.URL+"/oauth/token")
	t.Setenv("RUNE_PROVIDER", "codex")

	store := oauth.NewStore(filepath.Join(runeDir, "auth.json"))
	_ = store.Set("openai-codex", oauth.Credentials{
		AccessToken:  fakeAccessToken("acct_xyz"),
		RefreshToken: "RT",
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("output = %q", buf.String())
	}

	sessions, _ := os.ReadDir(filepath.Join(runeDir, "sessions"))
	if len(sessions) == 0 {
		t.Fatal("no session file written")
	}
}

func TestRunPrompt_HitsOllamaAndStreamsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("auth = %q, want empty", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ollama\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "ollama", "qwen3:4b", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ollama") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_UsesSavedRunpodEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+strings.Repeat("r", 24) {
			t.Errorf("auth = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"runpod\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_RUNPOD_API_KEY", strings.Repeat("r", 24))
	settings := map[string]any{
		"provider":        "runpod",
		"runpod_model":    "custom/model",
		"runpod_endpoint": srv.URL,
	}
	b, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(runeDir, "settings.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "runpod") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_OllamaSendsAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ollama-token" {
			t.Errorf("auth = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ollama-auth\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)
	t.Setenv("RUNE_OLLAMA_API_KEY", "ollama-token")

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "ollama", "qwen3:4b", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ollama-auth") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_HitsGroqAndStreamsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+strings.Repeat("g", 24) {
			t.Errorf("auth = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"groq\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_GROQ_ENDPOINT", srv.URL)
	t.Setenv("RUNE_GROQ_API_KEY", strings.Repeat("g", 24))

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "groq", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "groq") {
		t.Fatalf("output = %q", buf.String())
	}
}
