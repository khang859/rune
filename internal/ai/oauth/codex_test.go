package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBuildAuthorizeURL_ContainsRequiredParams(t *testing.T) {
	p, _ := GeneratePKCE()
	state, _ := GenerateState()
	u := BuildAuthorizeURL(state, p.Challenge)
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	checks := map[string]string{
		"client_id":             CodexClientID,
		"response_type":         "code",
		"redirect_uri":          CodexRedirectURI,
		"code_challenge":        p.Challenge,
		"code_challenge_method": "S256",
		"state":                 state,
	}
	for k, want := range checks {
		if q.Get(k) != want {
			t.Fatalf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	if !strings.Contains(q.Get("scope"), "offline_access") {
		t.Fatalf("scope missing offline_access: %q", q.Get("scope"))
	}
}

func TestExchangeCode_ParsesTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") != "authorization_code" {
			t.Fatalf("grant_type = %q", r.PostForm.Get("grant_type"))
		}
		if r.PostForm.Get("code") != "thecode" {
			t.Fatalf("code = %q", r.PostForm.Get("code"))
		}
		if r.PostForm.Get("code_verifier") != "v1" {
			t.Fatalf("code_verifier = %q", r.PostForm.Get("code_verifier"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "AT",
			"refresh_token": "RT",
			"expires_in":    3600,
			"id_token":      "fake.jwt.token",
		})
	}))
	defer srv.Close()

	creds, err := ExchangeCode(context.Background(), srv.URL+"/oauth/token", "thecode", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessToken != "AT" || creds.RefreshToken != "RT" {
		t.Fatalf("creds = %#v", creds)
	}
	if time.Until(creds.ExpiresAt) < 30*time.Minute {
		t.Fatalf("expires_at not in future: %v", creds.ExpiresAt)
	}
}

func TestRefreshToken_ParsesNewTokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("grant_type") != "refresh_token" {
			t.Fatal("wrong grant")
		}
		if r.PostForm.Get("refresh_token") != "RTOLD" {
			t.Fatal("wrong refresh")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "AT2",
			"refresh_token": "RTNEW",
			"expires_in":    3600,
		})
	}))
	defer srv.Close()

	creds, err := RefreshToken(context.Background(), srv.URL+"/oauth/token", "RTOLD")
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessToken != "AT2" || creds.RefreshToken != "RTNEW" {
		t.Fatalf("creds = %#v", creds)
	}
}

func TestAccountIDFromAccessToken_ExtractsClaim(t *testing.T) {
	payload := map[string]any{
		CodexJWTClaimPath: map[string]any{"chatgpt_account_id": "acct_123"},
	}
	pb, _ := json.Marshal(payload)
	tok := fmt.Sprintf("h.%s.s", base64.RawURLEncoding.EncodeToString(pb))
	id, err := AccountIDFromAccessToken(tok)
	if err != nil {
		t.Fatal(err)
	}
	if id != "acct_123" {
		t.Fatalf("id = %q", id)
	}
}

func TestAccountIDFromAccessToken_MissingClaim(t *testing.T) {
	pb, _ := json.Marshal(map[string]any{"sub": "x"})
	tok := fmt.Sprintf("h.%s.s", base64.RawURLEncoding.EncodeToString(pb))
	if _, err := AccountIDFromAccessToken(tok); err == nil {
		t.Fatal("expected error")
	}
}

func TestExchangeCode_PropagatesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	_, err := ExchangeCode(context.Background(), srv.URL+"/oauth/token", "x", "y")
	if err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("err = %v", err)
	}
}
