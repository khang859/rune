package oauth

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStore_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rune")
	p := filepath.Join(dir, "auth.json")
	st := NewStore(p)
	creds := Credentials{
		AccessToken:  "a1",
		RefreshToken: "r1",
		ExpiresAt:    time.Unix(1700000000, 0).UTC(),
		Account:      "user@example.com",
	}
	if err := st.Set("openai-codex", creds); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get("openai-codex")
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "a1" || got.RefreshToken != "r1" || got.Account != "user@example.com" {
		t.Fatalf("got = %#v", got)
	}
	for _, path := range []string{p, p + ".lock"} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s permissions = %o, want 600", path, got)
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", got)
	}
}

func TestStore_MigratesExistingDirPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rune")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	st := NewStore(filepath.Join(dir, "auth.json"))
	if err := st.Set("openai-codex", Credentials{AccessToken: "a"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", got)
	}
}

func TestStore_NoCredsForProvider(t *testing.T) {
	p := filepath.Join(t.TempDir(), "auth.json")
	st := NewStore(p)
	if _, err := st.Get("openai-codex"); err == nil {
		t.Fatal("expected error when no creds saved")
	}
}

func TestStore_ConcurrentSetIsSerialized(t *testing.T) {
	p := filepath.Join(t.TempDir(), "auth.json")
	st1 := NewStore(p)
	st2 := NewStore(p)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_ = st1.Set("a", Credentials{AccessToken: "x"})
		}(i)
		go func(i int) {
			defer wg.Done()
			_ = st2.Set("b", Credentials{AccessToken: "y"})
		}(i)
	}
	wg.Wait()

	a, err := st1.Get("a")
	if err != nil || a.AccessToken != "x" {
		t.Fatalf("a missing: %v %v", a, err)
	}
	b, err := st1.Get("b")
	if err != nil || b.AccessToken != "y" {
		t.Fatalf("b missing: %v %v", b, err)
	}
}
