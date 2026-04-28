package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai/oauth"
)

func TestRunPrompt_HitsCodexAndStreamsText(t *testing.T) {
	sse := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"event: response.completed\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{}}}\n\n"

	codex := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	store := oauth.NewStore(filepath.Join(runeDir, "auth.json"))
	_ = store.Set("openai-codex", oauth.Credentials{
		AccessToken:  "AT",
		RefreshToken: "RT",
		ExpiresAt:    time.Now().Add(time.Hour),
	})

	var buf bytes.Buffer
	if err := runPrompt(context.Background(), "say hi", &buf); err != nil {
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
