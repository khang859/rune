package oauth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const codexProviderKey = "openai-codex"

type CodexSource struct {
	Store    *Store
	TokenURL string

	mu sync.Mutex
}

func (s *CodexSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	creds, err := s.Store.Get(codexProviderKey)
	if err != nil {
		return "", err
	}
	if time.Until(creds.ExpiresAt) > 5*time.Minute {
		return creds.AccessToken, nil
	}
	return s.refreshLocked(ctx, creds)
}

// AccountID returns the chatgpt_account_id claim from the current access
// token. Codex requires it as the chatgpt-account-id header.
func (s *CodexSource) AccountID(ctx context.Context) (string, error) {
	tok, err := s.Token(ctx)
	if err != nil {
		return "", err
	}
	return AccountIDFromAccessToken(tok)
}

func (s *CodexSource) Refresh(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	creds, err := s.Store.Get(codexProviderKey)
	if err != nil {
		return err
	}
	_, err = s.refreshLocked(ctx, creds)
	return err
}

func (s *CodexSource) refreshLocked(ctx context.Context, creds Credentials) (string, error) {
	if creds.RefreshToken == "" {
		return "", fmt.Errorf("no refresh token; run /login")
	}
	new, err := RefreshToken(ctx, s.TokenURL, creds.RefreshToken)
	if err != nil {
		return "", err
	}
	if new.RefreshToken == "" {
		new.RefreshToken = creds.RefreshToken
	}
	new.Account = creds.Account
	if err := s.Store.Set(codexProviderKey, new); err != nil {
		return "", err
	}
	return new.AccessToken, nil
}
