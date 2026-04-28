package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	CodexClientID         = "app_EMoamEEZ73f0CkXaXp7hrann"
	CodexAuthorizeURL     = "https://auth.openai.com/oauth/authorize"
	CodexTokenURL         = "https://auth.openai.com/oauth/token"
	CodexCallbackPort     = 1455
	CodexRedirectURI      = "http://localhost:1455/auth/callback"
	CodexScope            = "openid profile email offline_access"
	CodexResponsesBaseURL = "https://chatgpt.com/backend-api"
	CodexResponsesPath    = "/codex/responses"
)

func BuildAuthorizeURL(state, challenge string) string {
	q := url.Values{}
	q.Set("client_id", CodexClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", CodexRedirectURI)
	q.Set("scope", CodexScope)
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return CodexAuthorizeURL + "?" + q.Encode()
}

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token,omitempty"`
}

func ExchangeCode(ctx context.Context, tokenURL, code, verifier string) (Credentials, error) {
	body := url.Values{}
	body.Set("grant_type", "authorization_code")
	body.Set("client_id", CodexClientID)
	body.Set("code", code)
	body.Set("redirect_uri", CodexRedirectURI)
	body.Set("code_verifier", verifier)
	return postToken(ctx, tokenURL, body)
}

func RefreshToken(ctx context.Context, tokenURL, refreshToken string) (Credentials, error) {
	body := url.Values{}
	body.Set("grant_type", "refresh_token")
	body.Set("client_id", CodexClientID)
	body.Set("refresh_token", refreshToken)
	body.Set("scope", CodexScope)
	return postToken(ctx, tokenURL, body)
}

func postToken(ctx context.Context, tokenURL string, body url.Values) (Credentials, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return Credentials{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Credentials{}, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return Credentials{}, fmt.Errorf("token endpoint %d: %s", resp.StatusCode, string(rb))
	}
	var tr tokenResp
	if err := json.Unmarshal(rb, &tr); err != nil {
		return Credentials{}, fmt.Errorf("parse token: %w; body=%s", err, string(rb))
	}
	if tr.ExpiresIn == 0 {
		tr.ExpiresIn = 3600
	}
	creds := Credentials{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}
	if tr.IDToken != "" {
		if email := emailFromIDToken(tr.IDToken); email != "" {
			creds.Account = email
		}
	}
	return creds, nil
}

func emailFromIDToken(t string) string {
	parts := strings.Split(t, ".")
	if len(parts) < 2 {
		return ""
	}
	seg := parts[1]
	pad := 4 - len(seg)%4
	if pad != 4 {
		seg += strings.Repeat("=", pad)
	}
	seg = strings.ReplaceAll(seg, "-", "+")
	seg = strings.ReplaceAll(seg, "_", "/")
	raw, err := base64Decode(seg)
	if err != nil {
		return ""
	}
	var payload struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return payload.Email
}

func base64Decode(s string) ([]byte, error) {
	return jsonStdBase64Decode(s)
}
