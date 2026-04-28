package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestSource_RefreshesExpiredAccess(t *testing.T) {
	refreshed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshed = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "AT-NEW",
			"refresh_token": "RT-NEW",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	store := NewStore(filepath.Join(t.TempDir(), "auth.json"))
	_ = store.Set("openai-codex", Credentials{
		AccessToken:  "AT-OLD",
		RefreshToken: "RT-OLD",
		ExpiresAt:    time.Now().Add(-1 * time.Minute),
	})

	src := &CodexSource{Store: store, TokenURL: srv.URL + "/oauth/token"}
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "AT-NEW" {
		t.Fatalf("token = %q", tok)
	}
	if !refreshed {
		t.Fatal("refresh did not occur")
	}
	creds, _ := store.Get("openai-codex")
	if creds.AccessToken != "AT-NEW" || creds.RefreshToken != "RT-NEW" {
		t.Fatalf("not persisted: %#v", creds)
	}
}

func TestSource_FreshTokenSkipsRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not refresh when token is fresh")
	}))
	defer srv.Close()

	store := NewStore(filepath.Join(t.TempDir(), "auth.json"))
	_ = store.Set("openai-codex", Credentials{
		AccessToken: "AT-FRESH",
		ExpiresAt:   time.Now().Add(30 * time.Minute),
	})
	src := &CodexSource{Store: store, TokenURL: srv.URL + "/oauth/token"}
	tok, _ := src.Token(context.Background())
	if tok != "AT-FRESH" {
		t.Fatalf("token = %q", tok)
	}
}
