package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestRunLoginFlow_HappyPath(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "ATX",
			"refresh_token": "RTX",
			"expires_in":    3600,
		})
	}))
	defer tokenSrv.Close()

	cfg := LoginConfig{
		AuthorizeURL: "http://localhost/ignored",
		TokenURL:     tokenSrv.URL + "/oauth/token",
		Port:         0,
		OpenBrowser:  func(string) error { return nil },
	}

	flow, err := StartLogin(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()

	cb := flow.CallbackURL() + "?code=THECODE&state=" + url.QueryEscape(flow.State())
	go func() {
		time.Sleep(50 * time.Millisecond)
		resp, err := http.Get(cb)
		if err != nil {
			t.Errorf("callback get: %v", err)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}()

	creds, err := flow.Wait(context.Background(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if creds.AccessToken != "ATX" {
		t.Fatalf("access = %q", creds.AccessToken)
	}
}

func TestRunLoginFlow_RejectsStateMismatch(t *testing.T) {
	cfg := LoginConfig{
		TokenURL:    "http://example.invalid",
		OpenBrowser: func(string) error { return nil },
	}
	flow, err := StartLogin(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer flow.Close()

	cb := flow.CallbackURL() + "?code=X&state=WRONG"
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = http.Get(cb)
	}()

	_, err = flow.Wait(context.Background(), 1*time.Second)
	if err == nil {
		t.Fatal("expected state-mismatch error")
	}
}
