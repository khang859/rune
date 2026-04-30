package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/khang859/rune/internal/ai/oauth"
	"github.com/khang859/rune/internal/config"
)

func runLogin(ctx context.Context, provider string) error {
	if provider == "groq" {
		return fmt.Errorf("groq uses an API key; set GROQ_API_KEY or RUNE_GROQ_API_KEY, or save it from /settings")
	}
	if provider != "codex" {
		return fmt.Errorf("unknown login provider %q (supported: codex; groq uses API keys)", provider)
	}
	if err := config.EnsureRuneDir(); err != nil {
		return err
	}

	tokenURL := oauth.CodexTokenURL
	if v := os.Getenv("RUNE_OAUTH_TOKEN_URL"); v != "" {
		tokenURL = v
	}
	authorizeURL := oauth.CodexAuthorizeURL
	if v := os.Getenv("RUNE_OAUTH_AUTHORIZE_URL"); v != "" {
		authorizeURL = v
	}

	cfg := oauth.LoginConfig{
		TokenURL:    tokenURL,
		Port:        oauth.CodexCallbackPort,
		OpenBrowser: openBrowser,
	}
	flow, err := oauth.StartLogin(cfg)
	if err != nil {
		return fmt.Errorf("start login: %w", err)
	}
	defer flow.Close()

	full := authorizeURL + "?" + queryFromAuthorize(flow.State(), flow.Challenge())
	cfg.AuthorizeURL = full
	fmt.Println("Open this URL in your browser:")
	fmt.Println(full)
	fmt.Println("Or it should open automatically.")
	_ = openBrowser(full)

	creds, err := flow.Wait(ctx, 5*time.Minute)
	if err != nil {
		return err
	}
	store := oauth.NewStore(config.AuthPath())
	if err := store.Set("openai-codex", creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	fmt.Println("Logged in.", "account:", creds.Account)
	return nil
}

func queryFromAuthorize(state, challenge string) string {
	return "client_id=" + oauth.CodexClientID +
		"&response_type=code" +
		"&redirect_uri=" + oauth.CodexRedirectURI +
		"&scope=" + urlEncode(oauth.CodexScope) +
		"&state=" + state +
		"&code_challenge=" + challenge +
		"&code_challenge_method=S256"
}

func urlEncode(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z',
			c == '-', c == '_', c == '.', c == '~':
			out = append(out, c)
		default:
			out = append(out, '%')
			const hex = "0123456789ABCDEF"
			out = append(out, hex[c>>4], hex[c&0xf])
		}
	}
	return string(out)
}

func openBrowser(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}
