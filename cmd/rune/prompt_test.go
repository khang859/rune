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
	if err := runPrompt(context.Background(), "say hi", "", "", "", "", "", &buf); err != nil {
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

func TestRunPrompt_UsesCodexProfileEndpoint(t *testing.T) {
	sse := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"profile-codex\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{}}}\n\n"

	codex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer codex.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	settings := map[string]any{
		"provider":       "codex",
		"active_profile": "codex-test",
		"profiles": []map[string]any{{
			"id":       "codex-test",
			"provider": "codex",
			"endpoint": codex.URL + "/codex/responses",
			"model":    "gpt-5.5",
		}},
	}
	b, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(runeDir, "settings.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	store := oauth.NewStore(filepath.Join(runeDir, "auth.json"))
	_ = store.Set("openai-codex", oauth.Credentials{
		AccessToken:  fakeAccessToken("acct_xyz"),
		RefreshToken: "RT",
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "", "", "", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "profile-codex") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_HitsOllamaAndStreamsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("auth = %q, want empty", got)
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ollama"},"done":true,"done_reason":"stop"}` + "\n"))
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "ollama", "qwen3:4b", "", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ollama") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_NoAttachSkipsFileResolution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ok"},"done":true,"done_reason":"stop"}` + "\n"))
	}))
	defer srv.Close()

	t.Setenv("RUNE_DIR", t.TempDir())
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)

	// A path-like token in the prompt is auto-attached; the missing file yields a
	// "(could not attach …)" warning on the writer. RUNE_NO_ATTACH must suppress it
	// so a bulk-text caller's prompt is treated literally.
	const prompt = "summarize the session; it edited missing-xyz.ts"

	t.Run("default resolves and warns", func(t *testing.T) {
		var buf bytes.Buffer
		if err := runPrompt(context.Background(), prompt, "ollama", "qwen3:4b", "", "", "", &buf); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "could not attach missing-xyz.ts") {
			t.Fatalf("expected attach warning, output = %q", buf.String())
		}
	})

	t.Run("RUNE_NO_ATTACH suppresses resolution", func(t *testing.T) {
		t.Setenv("RUNE_NO_ATTACH", "1")
		var buf bytes.Buffer
		if err := runPrompt(context.Background(), prompt, "ollama", "qwen3:4b", "", "", "", &buf); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buf.String(), "could not attach") {
			t.Fatalf("RUNE_NO_ATTACH should suppress attach warnings, output = %q", buf.String())
		}
		if !strings.Contains(buf.String(), "ok") {
			t.Fatalf("assistant text should still stream, output = %q", buf.String())
		}
	})
}

func TestRunPrompt_CanceledContextCancelsProviderRequest(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-releaseHandler
	}))
	defer srv.Close()
	defer close(releaseHandler)

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var buf bytes.Buffer
	go func() {
		done <- runPrompt(ctx, "say hi", "ollama", "qwen3:4b", "", "", "", &buf)
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("provider request did not start")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runPrompt did not return after cancellation")
	}
	if strings.Contains(buf.String(), "ollama") {
		t.Fatalf("canceled prompt streamed response: %q", buf.String())
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
	if err := runPrompt(context.Background(), "say hi", "", "", "", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "runpod") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_UsesOllamaProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer profile-token" {
			t.Errorf("auth = %q", got)
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"profile-ollama"},"done":true,"done_reason":"stop"}` + "\n"))
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	settings := map[string]any{
		"provider":       "ollama",
		"active_profile": "gpu",
		"profiles": []map[string]any{{
			"id":       "gpu",
			"name":     "GPU",
			"provider": "ollama",
			"endpoint": srv.URL,
			"model":    "qwen3:4b",
		}},
	}
	b, _ := json.Marshal(settings)
	if err := os.WriteFile(filepath.Join(runeDir, "settings.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	secrets := map[string]any{"profile_api_keys": map[string]string{"gpu": "profile-token"}}
	b, _ = json.Marshal(secrets)
	if err := os.WriteFile(filepath.Join(runeDir, "secrets.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "", "", "", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "profile-ollama") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_OllamaSendsAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ollama-token" {
			t.Errorf("auth = %q", got)
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ollama-auth"},"done":true,"done_reason":"stop"}` + "\n"))
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)
	t.Setenv("RUNE_OLLAMA_API_KEY", "ollama-token")

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "ollama", "qwen3:4b", "", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ollama-auth") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_HitsOpenRouterAndStreamsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+strings.Repeat("o", 24) {
			t.Errorf("auth = %q", got)
		}
		if got := r.Header.Get("X-OpenRouter-Title"); got != "rune" {
			t.Errorf("X-OpenRouter-Title = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if got := body["model"]; got != "anthropic/claude-sonnet-4.5" {
			t.Errorf("model = %v", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"openrouter\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_OPENROUTER_ENDPOINT", srv.URL)
	t.Setenv("RUNE_OPENROUTER_API_KEY", strings.Repeat("o", 24))

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "openrouter", "anthropic/claude-sonnet-4.5", "", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "openrouter") {
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
	if err := runPrompt(context.Background(), "say hi", "groq", "", "", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "groq") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_LoadsSkillsIntoSystemPrompt(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		gotBody = b.String()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("RUNE_DIR", tmp)
	t.Setenv("HOME", tmp)
	t.Setenv("RUNE_OPENROUTER_ENDPOINT", srv.URL)
	t.Setenv("RUNE_OPENROUTER_API_KEY", strings.Repeat("o", 24))

	skillsDir := filepath.Join(tmp, ".rune", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const marker = "call-kanban-heartbeat-every-turn"
	if err := os.WriteFile(filepath.Join(skillsDir, "kanban.md"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "openrouter", "anthropic/claude-sonnet-4.5", "", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, marker) {
		t.Fatalf("skill body not sent to provider; request body = %q", gotBody)
	}
}

func TestRunPrompt_MCPStartFailureDoesNotAbort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ollama"},"done":true,"done_reason":"stop"}` + "\n"))
	}))
	defer srv.Close()

	tmp := t.TempDir()
	t.Setenv("RUNE_DIR", tmp)
	t.Setenv("HOME", tmp)
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)

	mcpJSON := `{"servers":{"bad":{"command":"rune-nonexistent-binary-xyz"}}}`
	if err := os.WriteFile(filepath.Join(tmp, "mcp.json"), []byte(mcpJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", "ollama", "qwen3:4b", "", "", "", &buf); err != nil {
		t.Fatalf("runPrompt should tolerate MCP startup failure: %v", err)
	}
	if !strings.Contains(buf.String(), "ollama") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunPrompt_ResumeContinuesSavedSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"ollama"},"done":true,"done_reason":"stop"}` + "\n"))
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "first turn", "ollama", "qwen3:4b", "", "", "", &buf); err != nil {
		t.Fatal(err)
	}
	sessions, err := os.ReadDir(filepath.Join(runeDir, "sessions"))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions after first turn = %v, err = %v", sessions, err)
	}
	id := strings.TrimSuffix(sessions[0].Name(), ".json")

	if err := runPrompt(context.Background(), "second turn", "ollama", "qwen3:4b", "", "", id, &buf); err != nil {
		t.Fatal(err)
	}
	sessions, _ = os.ReadDir(filepath.Join(runeDir, "sessions"))
	if len(sessions) != 1 {
		t.Fatalf("resume created a new session; sessions = %d, want 1", len(sessions))
	}
	raw, err := os.ReadFile(filepath.Join(runeDir, "sessions", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"first turn", "second turn"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("session file missing %q", want)
		}
	}
}

func TestRunPrompt_ResumeUnknownSessionFails(t *testing.T) {
	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)

	var buf bytes.Buffer
	err := runPrompt(context.Background(), "hi", "ollama", "qwen3:4b", "", "", "missing123", &buf)
	if err == nil || !strings.Contains(err.Error(), "resume session") {
		t.Fatalf("err = %v, want resume session error", err)
	}
}

func TestRunPrompt_TurnErrorReturnsError(t *testing.T) {
	// HTTP 400 classifies as ai.ErrFatal in the ollama provider — not retried,
	// so the agent loop emits a single TurnError.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	t.Setenv("RUNE_OLLAMA_ENDPOINT", srv.URL)

	var buf bytes.Buffer
	err := runPrompt(context.Background(), "say hi", "ollama", "qwen3:4b", "", "", "", &buf)
	if err == nil {
		t.Fatalf("want non-nil error; transcript = %q", buf.String())
	}
	if !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("err = %v, want provider status 400 error", err)
	}
	if !strings.Contains(buf.String(), "[error:") {
		t.Fatalf("transcript missing [error: ...] line: %q", buf.String())
	}
	// The session must still be saved before the error is returned.
	sessions, _ := os.ReadDir(filepath.Join(runeDir, "sessions"))
	if len(sessions) == 0 {
		t.Fatal("session not saved before error return")
	}
}
