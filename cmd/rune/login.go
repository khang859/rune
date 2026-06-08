package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/khang859/rune/internal/ai/oauth"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/providers"
)

// runLoginInteractive is `rune login` with no provider argument: an interactive
// chooser that doubles as a CLI-level provider switch. It lists every provider,
// persists the chosen one as the active provider, then either runs the OAuth
// flow (Codex) or reports what's needed to use the chosen provider.
func runLoginInteractive(ctx context.Context, in io.Reader, out io.Writer) error {
	if err := config.EnsureRuneDir(); err != nil {
		return err
	}
	all := providers.All()
	settings, _ := config.LoadSettings(config.SettingsPath())
	current := strings.TrimSpace(settings.Provider)

	fmt.Fprintln(out, "Choose a provider:")
	for i, p := range all {
		active := " "
		if p.ID == current {
			active = "*"
		}
		fmt.Fprintf(out, " %s %d) %s\n", active, i+1, providerLoginLabel(p.ID))
	}
	fmt.Fprint(out, "> ")

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return fmt.Errorf("read selection: %w", err)
	}
	pick := strings.TrimSpace(line)
	choice, perr := strconv.Atoi(pick)
	if perr != nil || choice < 1 || choice > len(all) {
		return fmt.Errorf("invalid selection %q (enter 1-%d)", pick, len(all))
	}
	chosen := all[choice-1].ID

	// Persist the chosen provider as active so this doubles as a switcher.
	prev := settings.Provider
	settings.Provider = chosen
	settings.ActiveProfile = ""
	if err := config.SaveSettings(config.SettingsPath(), settings); err != nil {
		return fmt.Errorf("save provider selection: %w", err)
	}
	fmt.Fprintf(out, "Active provider set to %s.\n", providerDisplay(chosen))

	switch chosen {
	case providers.Codex:
		if err := runLogin(ctx, providers.Codex); err != nil {
			// Don't strand the user on Codex if the sign-in they just started
			// fails or is cancelled — restore their previous active provider.
			settings.Provider = prev
			_ = config.SaveSettings(config.SettingsPath(), settings)
			return err
		}
		return nil
	case providers.Groq, providers.Runpod, providers.OpenRouter:
		if providerHasKey(chosen) {
			fmt.Fprintf(out, "%s API key found — you're ready to go.\n", providerDisplay(chosen))
		} else {
			fmt.Fprintf(out, "%s needs an API key — add it from /settings in the TUI, or set the matching env var.\n", providerDisplay(chosen))
		}
		return nil
	case providers.Ollama:
		fmt.Fprintln(out, "Ollama runs locally — run `ollama serve` and `ollama pull <model>`, then start rune.")
		return nil
	default:
		return nil
	}
}

func providerLoginLabel(id string) string {
	switch id {
	case providers.Codex:
		return "Codex (browser sign-in)"
	case providers.Ollama:
		return "Ollama (local)"
	default:
		return providerDisplay(id) + " (API key)"
	}
}

func providerHasKey(id string) bool {
	store := config.NewSecretStore(config.SecretsPath())
	var key string
	switch id {
	case providers.Groq:
		key, _ = store.GroqAPIKey()
	case providers.Runpod:
		key, _ = store.RunpodAPIKey()
	case providers.OpenRouter:
		key, _ = store.OpenRouterAPIKey()
	}
	return strings.TrimSpace(key) != ""
}

func runLogin(ctx context.Context, provider string) error {
	if provider == "groq" {
		return fmt.Errorf("groq uses an API key; set GROQ_API_KEY or RUNE_GROQ_API_KEY, or save it from /settings")
	}
	if provider == "ollama" {
		return fmt.Errorf("ollama runs locally and does not use login; run `ollama serve` and `ollama pull <model>`, then use `rune --provider ollama --model <model>`")
	}
	if provider != "codex" {
		return fmt.Errorf("unknown login provider %q (supported: codex; groq uses API keys; ollama uses local models)", provider)
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
