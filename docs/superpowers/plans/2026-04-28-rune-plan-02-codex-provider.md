# rune Plan 02 — Codex Provider (OAuth + Streaming)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the OpenAI Codex provider so rune can run a real turn against `chatgpt.com/backend-api/codex/responses` using a ChatGPT Pro/Plus subscription via OAuth (PKCE). At the end, `rune --prompt "..."` produces real model output through the same agent loop from Plan 01.

**Architecture:** Two new packages. `internal/ai/oauth` does the PKCE dance (browser → localhost callback → token exchange → refresh) and persists `~/.rune/auth.json` under a file lock. `internal/ai/codex` is an `ai.Provider` that builds an SSE request to the Codex Responses endpoint, parses incoming events, and emits `ai.Event`s. CLI gets two new modes: `rune login codex` and `rune --prompt`.

**Tech Stack:** Standard library `net/http`, `crypto/rand`, `crypto/sha256`, `encoding/base64`. SSE parser is hand-rolled (no third-party SSE library — the format is simple). File locking via `golang.org/x/sys/unix.Flock` (the only third-party dep this plan adds).

**Spec:** `docs/superpowers/specs/2026-04-28-rune-coding-agent-design.md`

**Reference for wire format:** `reference/pi-mono/packages/ai/src/utils/oauth/openai-codex.ts` and `reference/pi-mono/packages/ai/src/providers/openai-codex-responses.ts`.

---

## File Structure

```
internal/ai/
├── oauth/
│   ├── pkce.go              # PKCE pair + state generator
│   ├── pkce_test.go
│   ├── codex.go             # Codex OAuth: authorize URL, callback server, token exchange, refresh
│   ├── codex_test.go        # against stub authorize/token servers
│   └── store.go             # auth.json read/write with file lock
│   └── store_test.go
└── codex/
    ├── codex.go             # ai.Provider implementation
    ├── codex_test.go
    ├── sse.go               # streaming SSE parser
    ├── sse_test.go
    ├── request.go           # Request → Codex Responses payload
    ├── request_test.go
    └── testdata/
        ├── stream_text_only.sse
        ├── stream_tool_call.sse
        └── stream_overflow.sse
cmd/rune/
├── login.go                 # `rune login codex`
├── login_test.go
├── prompt.go                # `rune --prompt "..."` one-shot mode
└── prompt_test.go
```

---

## Constants (locked)

```go
// from reference/pi-mono/packages/ai/src/utils/oauth/openai-codex.ts and openai-codex-responses.ts

const (
    CodexClientID         = "app_EMoamEEZ73f0CkXaXp7hrann"
    CodexAuthorizeURL     = "https://auth.openai.com/oauth/authorize"
    CodexTokenURL         = "https://auth.openai.com/oauth/token"
    CodexCallbackPort     = 1455
    CodexRedirectURI      = "http://localhost:1455/auth/callback"
    CodexScope            = "openid profile email offline_access"
    CodexJWTClaimPath     = "https://api.openai.com/auth"
    CodexResponsesBaseURL = "https://chatgpt.com/backend-api"
    CodexResponsesPath    = "/codex/responses"
)
```

---

## Task 1: PKCE generator

**Files:**
- Create: `internal/ai/oauth/pkce.go`
- Create: `internal/ai/oauth/pkce_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ai/oauth/pkce_test.go
package oauth

import (
    "crypto/sha256"
    "encoding/base64"
    "strings"
    "testing"
)

func TestGeneratePKCE_VerifierIsUniqueAndUrlSafe(t *testing.T) {
    a, err := GeneratePKCE()
    if err != nil { t.Fatal(err) }
    b, _ := GeneratePKCE()
    if a.Verifier == b.Verifier {
        t.Fatal("verifiers must differ between calls")
    }
    if strings.ContainsAny(a.Verifier, "+/=") {
        t.Fatalf("verifier not url-safe: %q", a.Verifier)
    }
}

func TestGeneratePKCE_ChallengeIsS256OfVerifier(t *testing.T) {
    p, _ := GeneratePKCE()
    sum := sha256.Sum256([]byte(p.Verifier))
    want := base64.RawURLEncoding.EncodeToString(sum[:])
    if p.Challenge != want {
        t.Fatalf("challenge mismatch:\n got = %q\nwant = %q", p.Challenge, want)
    }
}

func TestGenerateState_DistinctBytes(t *testing.T) {
    s1, _ := GenerateState()
    s2, _ := GenerateState()
    if s1 == s2 {
        t.Fatal("states must differ")
    }
    if len(s1) < 16 {
        t.Fatalf("state too short: %q", s1)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/oauth/...`
Expected: FAIL — package does not compile.

- [ ] **Step 3: Implement**

```go
// internal/ai/oauth/pkce.go
package oauth

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
)

type PKCE struct {
    Verifier  string
    Challenge string
}

func GeneratePKCE() (PKCE, error) {
    b := make([]byte, 64)
    if _, err := rand.Read(b); err != nil {
        return PKCE{}, err
    }
    v := base64.RawURLEncoding.EncodeToString(b)
    sum := sha256.Sum256([]byte(v))
    return PKCE{Verifier: v, Challenge: base64.RawURLEncoding.EncodeToString(sum[:])}, nil
}

func GenerateState() (string, error) {
    b := make([]byte, 24)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return base64.RawURLEncoding.EncodeToString(b), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/oauth/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/oauth/pkce.go internal/ai/oauth/pkce_test.go
git commit -m "feat(oauth): pkce + state generator"
```

---

## Task 2: Auth storage (~/.rune/auth.json with file lock)

**Files:**
- Add dependency: `go get golang.org/x/sys`
- Create: `internal/ai/oauth/store.go`
- Create: `internal/ai/oauth/store_test.go`

- [ ] **Step 1: Add dependency**

Run: `go get golang.org/x/sys`

- [ ] **Step 2: Write the failing test**

```go
// internal/ai/oauth/store_test.go
package oauth

import (
    "path/filepath"
    "sync"
    "testing"
    "time"
)

func TestStore_RoundTrip(t *testing.T) {
    p := filepath.Join(t.TempDir(), "auth.json")
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

    // Both keys must be present in final file.
    a, err := st1.Get("a")
    if err != nil || a.AccessToken != "x" {
        t.Fatalf("a missing: %v %v", a, err)
    }
    b, err := st1.Get("b")
    if err != nil || b.AccessToken != "y" {
        t.Fatalf("b missing: %v %v", b, err)
    }
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/ai/oauth/ -run TestStore`
Expected: FAIL — Store undefined.

- [ ] **Step 4: Implement**

```go
// internal/ai/oauth/store.go
package oauth

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "golang.org/x/sys/unix"
)

type Credentials struct {
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token,omitempty"`
    ExpiresAt    time.Time `json:"expires_at"`
    Account      string    `json:"account,omitempty"`
}

type Store struct {
    path string
}

func NewStore(path string) *Store {
    return &Store{path: path}
}

type fileShape struct {
    Providers map[string]Credentials `json:"providers"`
}

func (s *Store) Get(provider string) (Credentials, error) {
    f, err := s.openLocked(unix.LOCK_SH)
    if err != nil {
        return Credentials{}, err
    }
    defer f.unlockAndClose()

    fs, err := f.read()
    if err != nil {
        return Credentials{}, err
    }
    c, ok := fs.Providers[provider]
    if !ok {
        return Credentials{}, fmt.Errorf("no credentials for %q", provider)
    }
    return c, nil
}

func (s *Store) Set(provider string, creds Credentials) error {
    f, err := s.openLocked(unix.LOCK_EX)
    if err != nil {
        return err
    }
    defer f.unlockAndClose()

    fs, err := f.read()
    if err != nil {
        return err
    }
    if fs.Providers == nil {
        fs.Providers = map[string]Credentials{}
    }
    fs.Providers[provider] = creds
    return f.writeAtomic(fs)
}

// ----- locked file handle -----

type lockedFile struct {
    path string
    f    *os.File
}

func (s *Store) openLocked(mode int) (*lockedFile, error) {
    if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
        return nil, err
    }
    // Use a sibling lock file so we lock even when auth.json doesn't exist.
    lockPath := s.path + ".lock"
    f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
    if err != nil {
        return nil, err
    }
    if err := unix.Flock(int(f.Fd()), mode); err != nil {
        f.Close()
        return nil, err
    }
    return &lockedFile{path: s.path, f: f}, nil
}

func (lf *lockedFile) unlockAndClose() {
    _ = unix.Flock(int(lf.f.Fd()), unix.LOCK_UN)
    _ = lf.f.Close()
}

func (lf *lockedFile) read() (fileShape, error) {
    var fs fileShape
    b, err := os.ReadFile(lf.path)
    if err != nil {
        if os.IsNotExist(err) {
            return fileShape{Providers: map[string]Credentials{}}, nil
        }
        return fs, err
    }
    if len(b) == 0 {
        return fileShape{Providers: map[string]Credentials{}}, nil
    }
    if err := json.Unmarshal(b, &fs); err != nil {
        return fs, err
    }
    return fs, nil
}

func (lf *lockedFile) writeAtomic(fs fileShape) error {
    b, err := json.MarshalIndent(fs, "", "  ")
    if err != nil {
        return err
    }
    tmp := lf.path + ".tmp"
    if err := os.WriteFile(tmp, b, 0o600); err != nil {
        return err
    }
    return os.Rename(tmp, lf.path)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/ai/oauth/ -run TestStore`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/ai/oauth/store.go internal/ai/oauth/store_test.go go.mod go.sum
git commit -m "feat(oauth): credentials store with file lock"
```

---

## Task 3: Codex authorize URL + token exchange (against stub server)

**Files:**
- Create: `internal/ai/oauth/codex.go`
- Create: `internal/ai/oauth/codex_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/ai/oauth/codex_test.go
package oauth

import (
    "context"
    "encoding/json"
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
    if err != nil { t.Fatal(err) }
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
    // Stub token server returns access/refresh/expires_in.
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/oauth/token" { t.Fatalf("path = %s", r.URL.Path) }
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
    if err != nil { t.Fatal(err) }
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
        if r.PostForm.Get("grant_type") != "refresh_token" { t.Fatal("wrong grant") }
        if r.PostForm.Get("refresh_token") != "RTOLD" { t.Fatal("wrong refresh") }
        _ = json.NewEncoder(w).Encode(map[string]any{
            "access_token":  "AT2",
            "refresh_token": "RTNEW",
            "expires_in":    3600,
        })
    }))
    defer srv.Close()

    creds, err := RefreshToken(context.Background(), srv.URL+"/oauth/token", "RTOLD")
    if err != nil { t.Fatal(err) }
    if creds.AccessToken != "AT2" || creds.RefreshToken != "RTNEW" {
        t.Fatalf("creds = %#v", creds)
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/oauth/ -run "Authorize|Exchange|Refresh"`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement**

```go
// internal/ai/oauth/codex.go
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
    // base64url decode payload
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

// indirection so tests do not need stdlib base64 import inline
func base64Decode(s string) ([]byte, error) {
    return jsonStdBase64Decode(s)
}
```

```go
// internal/ai/oauth/codex_decode.go
package oauth

import "encoding/base64"

func jsonStdBase64Decode(s string) ([]byte, error) {
    return base64.StdEncoding.DecodeString(s)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/oauth/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/oauth/codex.go internal/ai/oauth/codex_decode.go internal/ai/oauth/codex_test.go
git commit -m "feat(oauth): authorize URL builder + token exchange/refresh"
```

---

## Task 4: Codex login flow (callback server + browser open)

**Files:**
- Create: `internal/ai/oauth/login.go`
- Create: `internal/ai/oauth/login_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ai/oauth/login_test.go
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
    // Stub token server.
    tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _ = json.NewEncoder(w).Encode(map[string]any{
            "access_token":  "ATX",
            "refresh_token": "RTX",
            "expires_in":    3600,
        })
    }))
    defer tokenSrv.Close()

    cfg := LoginConfig{
        AuthorizeURL: "http://localhost/ignored", // we don't open the browser in tests
        TokenURL:     tokenSrv.URL + "/oauth/token",
        Port:         0, // pick a free port
        OpenBrowser:  func(string) error { return nil },
    }

    flow, err := StartLogin(cfg)
    if err != nil {
        t.Fatal(err)
    }
    defer flow.Close()

    // Hit the callback endpoint with a matching state.
    cb := flow.CallbackURL() + "?code=THECODE&state=" + url.QueryEscape(flow.State())
    go func() {
        time.Sleep(50 * time.Millisecond)
        resp, err := http.Get(cb)
        if err != nil { t.Errorf("callback get: %v", err) }
        if resp != nil { resp.Body.Close() }
    }()

    creds, err := flow.Wait(context.Background(), 2*time.Second)
    if err != nil { t.Fatal(err) }
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
    if err != nil { t.Fatal(err) }
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/oauth/ -run TestRunLoginFlow`
Expected: FAIL — `StartLogin` undefined.

- [ ] **Step 3: Implement**

```go
// internal/ai/oauth/login.go
package oauth

import (
    "context"
    "fmt"
    "net"
    "net/http"
    "net/url"
    "sync"
    "time"
)

type LoginConfig struct {
    AuthorizeURL string
    TokenURL     string
    Port         int                       // 0 = pick free port
    OpenBrowser  func(authURL string) error
}

type LoginFlow struct {
    cfg     LoginConfig
    pkce    PKCE
    state   string
    srv     *http.Server
    listen  net.Listener
    once    sync.Once
    result  chan loginResult
    cbPath  string
}

type loginResult struct {
    creds Credentials
    err   error
}

func StartLogin(cfg LoginConfig) (*LoginFlow, error) {
    pkce, err := GeneratePKCE()
    if err != nil { return nil, err }
    state, err := GenerateState()
    if err != nil { return nil, err }

    addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
    ln, err := net.Listen("tcp", addr)
    if err != nil { return nil, err }

    flow := &LoginFlow{
        cfg:    cfg,
        pkce:   pkce,
        state:  state,
        listen: ln,
        result: make(chan loginResult, 1),
        cbPath: "/auth/callback",
    }

    mux := http.NewServeMux()
    mux.HandleFunc(flow.cbPath, flow.handle)
    flow.srv = &http.Server{Handler: mux}
    go flow.srv.Serve(ln)

    if cfg.OpenBrowser != nil && cfg.AuthorizeURL != "" {
        // best-effort
        _ = cfg.OpenBrowser(cfg.AuthorizeURL)
    }
    return flow, nil
}

func (f *LoginFlow) State() string       { return f.state }
func (f *LoginFlow) Verifier() string    { return f.pkce.Verifier }
func (f *LoginFlow) Challenge() string   { return f.pkce.Challenge }
func (f *LoginFlow) CallbackURL() string {
    return "http://" + f.listen.Addr().String() + f.cbPath
}

func (f *LoginFlow) handle(w http.ResponseWriter, r *http.Request) {
    q := r.URL.Query()
    if q.Get("state") != f.state {
        http.Error(w, "state mismatch", http.StatusBadRequest)
        f.send(loginResult{err: fmt.Errorf("oauth state mismatch")})
        return
    }
    if errStr := q.Get("error"); errStr != "" {
        http.Error(w, errStr, http.StatusBadRequest)
        f.send(loginResult{err: fmt.Errorf("oauth error: %s", errStr)})
        return
    }
    code := q.Get("code")
    if code == "" {
        http.Error(w, "missing code", http.StatusBadRequest)
        f.send(loginResult{err: fmt.Errorf("missing code")})
        return
    }
    fmt.Fprint(w, "Login complete. You can close this tab.")

    creds, err := ExchangeCode(r.Context(), f.cfg.TokenURL, code, f.pkce.Verifier)
    f.send(loginResult{creds: creds, err: err})
}

func (f *LoginFlow) send(r loginResult) {
    select {
    case f.result <- r:
    default:
    }
}

func (f *LoginFlow) Wait(ctx context.Context, timeout time.Duration) (Credentials, error) {
    select {
    case r := <-f.result:
        return r.creds, r.err
    case <-time.After(timeout):
        return Credentials{}, fmt.Errorf("login timed out after %s", timeout)
    case <-ctx.Done():
        return Credentials{}, ctx.Err()
    }
}

func (f *LoginFlow) Close() {
    f.once.Do(func() {
        ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
        defer cancel()
        _ = f.srv.Shutdown(ctx)
    })
}

func openBrowserDefault(u string) error {
    // platform-aware browser open is added in main; tests inject their own.
    _, err := url.Parse(u)
    return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/oauth/ -run TestRunLoginFlow`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/oauth/login.go internal/ai/oauth/login_test.go
git commit -m "feat(oauth): callback server + login flow"
```

---

## Task 5: Codex SSE parser

**Files:**
- Create: `internal/ai/codex/sse.go`
- Create: `internal/ai/codex/sse_test.go`
- Create: `internal/ai/codex/testdata/stream_text_only.sse`
- Create: `internal/ai/codex/testdata/stream_tool_call.sse`
- Create: `internal/ai/codex/testdata/stream_overflow.sse`

> The SSE format Codex uses follows the OpenAI Responses streaming spec. Each event line is `event: <name>\ndata: <json>\n\n`. We only need a small subset of event types for v1: `response.output_text.delta`, `response.output_item.added` (for tool calls), `response.completed`, `response.failed`, `response.incomplete` (with `reason: context_length_exceeded`), and the wrapping `response.created`. Unknown event types are ignored.

- [ ] **Step 1: Write a representative fixture for text-only streaming**

```
# internal/ai/codex/testdata/stream_text_only.sse
event: response.created
data: {"type":"response.created","response":{"id":"r1","status":"in_progress"}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"hello"}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":" world"}

event: response.completed
data: {"type":"response.completed","response":{"id":"r1","status":"completed","usage":{"input_tokens":10,"output_tokens":2,"input_tokens_details":{"cached_tokens":0}}}}
```

- [ ] **Step 2: Write the failing test**

```go
// internal/ai/codex/sse_test.go
package codex

import (
    "context"
    "encoding/json"
    "os"
    "strings"
    "testing"

    "github.com/khang859/rune/internal/ai"
)

func collect(t *testing.T, ch <-chan ai.Event) []ai.Event {
    t.Helper()
    var out []ai.Event
    for e := range ch {
        out = append(out, e)
    }
    return out
}

func TestParseSSE_TextOnly(t *testing.T) {
    b, _ := os.ReadFile("testdata/stream_text_only.sse")
    out := make(chan ai.Event, 32)
    err := parseSSE(context.Background(), strings.NewReader(string(b)), out)
    close(out)
    if err != nil { t.Fatal(err) }
    evs := collect(t, out)

    var text strings.Builder
    var sawDone bool
    var usage ai.Usage
    for _, e := range evs {
        switch v := e.(type) {
        case ai.TextDelta:
            text.WriteString(v.Text)
        case ai.Usage:
            usage = v
        case ai.Done:
            sawDone = true
            if v.Reason != "stop" {
                t.Fatalf("done reason = %q", v.Reason)
            }
        }
    }
    if text.String() != "hello world" {
        t.Fatalf("text = %q", text.String())
    }
    if !sawDone {
        t.Fatal("missing Done")
    }
    if usage.Input != 10 || usage.Output != 2 {
        t.Fatalf("usage = %#v", usage)
    }
}

func TestParseSSE_ToolCall(t *testing.T) {
    b, _ := os.ReadFile("testdata/stream_tool_call.sse")
    out := make(chan ai.Event, 32)
    if err := parseSSE(context.Background(), strings.NewReader(string(b)), out); err != nil {
        t.Fatal(err)
    }
    close(out)
    evs := collect(t, out)
    var found bool
    for _, e := range evs {
        if c, ok := e.(ai.ToolCall); ok {
            found = true
            if c.Name != "read" {
                t.Fatalf("tool name = %q", c.Name)
            }
            var args map[string]string
            if err := json.Unmarshal(c.Args, &args); err != nil { t.Fatal(err) }
            if args["path"] != "/tmp/x" {
                t.Fatalf("args = %v", args)
            }
        }
    }
    if !found { t.Fatal("no ToolCall emitted") }
}

func TestParseSSE_ContextOverflow(t *testing.T) {
    b, _ := os.ReadFile("testdata/stream_overflow.sse")
    out := make(chan ai.Event, 32)
    if err := parseSSE(context.Background(), strings.NewReader(string(b)), out); err != nil {
        t.Fatal(err)
    }
    close(out)
    evs := collect(t, out)
    for _, e := range evs {
        if d, ok := e.(ai.Done); ok && d.Reason == "context_overflow" {
            return
        }
    }
    t.Fatal("missing Done{context_overflow}")
}
```

- [ ] **Step 3: Add the other fixtures**

```
# internal/ai/codex/testdata/stream_tool_call.sse
event: response.output_item.added
data: {"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","name":"read","arguments":"{\"path\":\"/tmp/x\"}"}}

event: response.completed
data: {"type":"response.completed","response":{"id":"r1","status":"completed","usage":{"input_tokens":1,"output_tokens":1}}}
```

```
# internal/ai/codex/testdata/stream_overflow.sse
event: response.incomplete
data: {"type":"response.incomplete","response":{"id":"r1","status":"incomplete","incomplete_details":{"reason":"context_length_exceeded"}}}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/ai/codex/...`
Expected: FAIL — package does not compile.

- [ ] **Step 5: Implement**

```go
// internal/ai/codex/sse.go
package codex

import (
    "bufio"
    "context"
    "encoding/json"
    "io"
    "strings"

    "github.com/khang859/rune/internal/ai"
)

// parseSSE reads SSE from r and emits ai.Event values to out.
// out is owned by the caller; parseSSE does NOT close it.
func parseSSE(ctx context.Context, r io.Reader, out chan<- ai.Event) error {
    scanner := bufio.NewScanner(r)
    scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

    var (
        eventName string
        dataBuf   strings.Builder
    )
    flush := func() error {
        if eventName == "" && dataBuf.Len() == 0 {
            return nil
        }
        data := dataBuf.String()
        eventName, dataBuf = "", strings.Builder{}
        return dispatchEvent(ctx, eventName, data, out)
    }
    _ = flush // keep for symmetry; we inline below

    for scanner.Scan() {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        line := scanner.Text()
        if line == "" {
            // blank line: end of event
            if dataBuf.Len() > 0 || eventName != "" {
                if err := dispatchEvent(ctx, eventName, dataBuf.String(), out); err != nil {
                    return err
                }
            }
            eventName = ""
            dataBuf.Reset()
            continue
        }
        if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ":") {
            continue
        }
        if strings.HasPrefix(line, "event:") {
            eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
            continue
        }
        if strings.HasPrefix(line, "data:") {
            if dataBuf.Len() > 0 {
                dataBuf.WriteByte('\n')
            }
            dataBuf.WriteString(strings.TrimPrefix(line, "data:"))
            // Trim a single leading space that SSE allows.
            // (we'll trim after concatenation)
            continue
        }
    }
    // dispatch any trailing event (some servers omit final blank line)
    if dataBuf.Len() > 0 || eventName != "" {
        if err := dispatchEvent(ctx, eventName, dataBuf.String(), out); err != nil {
            return err
        }
    }
    return scanner.Err()
}

type usageWire struct {
    InputTokens         int `json:"input_tokens"`
    OutputTokens        int `json:"output_tokens"`
    InputTokensDetails  struct {
        CachedTokens int `json:"cached_tokens"`
    } `json:"input_tokens_details"`
}

type respCompleted struct {
    Type     string `json:"type"`
    Response struct {
        Status string    `json:"status"`
        Usage  usageWire `json:"usage"`
    } `json:"response"`
}

type respIncomplete struct {
    Type     string `json:"type"`
    Response struct {
        IncompleteDetails struct {
            Reason string `json:"reason"`
        } `json:"incomplete_details"`
    } `json:"response"`
}

type respFailed struct {
    Type  string `json:"type"`
    Error struct {
        Message string `json:"message"`
        Code    string `json:"code"`
    } `json:"error"`
}

type textDelta struct {
    Type  string `json:"type"`
    Delta string `json:"delta"`
}

type itemAdded struct {
    Type string `json:"type"`
    Item struct {
        Type      string `json:"type"`
        ID        string `json:"id"`
        Name      string `json:"name"`
        Arguments string `json:"arguments"`
    } `json:"item"`
}

func dispatchEvent(ctx context.Context, name, data string, out chan<- ai.Event) error {
    data = strings.TrimSpace(data)
    if data == "" {
        return nil
    }
    switch name {
    case "response.output_text.delta":
        var d textDelta
        if err := json.Unmarshal([]byte(data), &d); err != nil {
            return nil // skip malformed delta lines
        }
        return send(ctx, out, ai.TextDelta{Text: d.Delta})

    case "response.output_item.added":
        var ia itemAdded
        if err := json.Unmarshal([]byte(data), &ia); err != nil {
            return nil
        }
        if ia.Item.Type == "function_call" {
            return send(ctx, out, ai.ToolCall{
                ID:   ia.Item.ID,
                Name: ia.Item.Name,
                Args: json.RawMessage(ia.Item.Arguments),
            })
        }
        return nil

    case "response.completed":
        var rc respCompleted
        if err := json.Unmarshal([]byte(data), &rc); err != nil {
            return nil
        }
        if err := send(ctx, out, ai.Usage{
            Input:     rc.Response.Usage.InputTokens,
            Output:    rc.Response.Usage.OutputTokens,
            CacheRead: rc.Response.Usage.InputTokensDetails.CachedTokens,
        }); err != nil {
            return err
        }
        return send(ctx, out, ai.Done{Reason: "stop"})

    case "response.incomplete":
        var ri respIncomplete
        _ = json.Unmarshal([]byte(data), &ri)
        if ri.Response.IncompleteDetails.Reason == "context_length_exceeded" {
            return send(ctx, out, ai.Done{Reason: "context_overflow"})
        }
        return send(ctx, out, ai.Done{Reason: "max_tokens"})

    case "response.failed":
        var rf respFailed
        _ = json.Unmarshal([]byte(data), &rf)
        return send(ctx, out, ai.StreamError{
            Err:       errString(rf.Error.Message),
            Retryable: false,
        })
    }
    return nil
}

func send(ctx context.Context, out chan<- ai.Event, e ai.Event) error {
    select {
    case <-ctx.Done():
        return ctx.Err()
    case out <- e:
        return nil
    }
}

type errString string

func (e errString) Error() string { return string(e) }
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/ai/codex/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/ai/codex/sse.go internal/ai/codex/sse_test.go internal/ai/codex/testdata/
git commit -m "feat(codex): SSE parser for Responses API"
```

---

## Task 6: Codex request builder

**Files:**
- Create: `internal/ai/codex/request.go`
- Create: `internal/ai/codex/request_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ai/codex/request_test.go
package codex

import (
    "encoding/json"
    "strings"
    "testing"

    "github.com/khang859/rune/internal/ai"
)

func TestBuildPayload_IncludesMessagesAndTools(t *testing.T) {
    req := ai.Request{
        Model:    "gpt-5",
        System:   "you are helpful",
        Messages: []ai.Message{
            {Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}},
        },
        Tools: []ai.ToolSpec{
            {Name: "read", Description: "Read file", Schema: json.RawMessage(`{"type":"object"}`)},
        },
        Reasoning: ai.ReasoningConfig{Effort: "medium"},
    }
    b, err := buildPayload(req)
    if err != nil { t.Fatal(err) }
    s := string(b)
    for _, want := range []string{
        `"model":"gpt-5"`,
        `"instructions":"you are helpful"`,
        `"input"`,
        `"tools"`,
        `"reasoning":{"effort":"medium"`,
    } {
        if !strings.Contains(s, want) {
            t.Fatalf("payload missing %q:\n%s", want, s)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/codex/ -run TestBuildPayload`
Expected: FAIL — buildPayload undefined.

- [ ] **Step 3: Implement**

```go
// internal/ai/codex/request.go
package codex

import (
    "encoding/json"

    "github.com/khang859/rune/internal/ai"
)

type payload struct {
    Model         string             `json:"model"`
    Stream        bool               `json:"stream"`
    Instructions  string             `json:"instructions,omitempty"`
    Input         []inputItem        `json:"input"`
    Tools         []payloadTool      `json:"tools,omitempty"`
    ToolChoice    string             `json:"tool_choice,omitempty"`
    ParallelTools bool               `json:"parallel_tool_calls,omitempty"`
    Reasoning     map[string]any     `json:"reasoning,omitempty"`
    Store         bool               `json:"store"`
}

type inputItem struct {
    Type    string          `json:"type"`
    Role    string          `json:"role,omitempty"`
    Content []inputContent  `json:"content,omitempty"`
    // function_call / function_call_output items:
    CallID    string          `json:"call_id,omitempty"`
    Name      string          `json:"name,omitempty"`
    Arguments string          `json:"arguments,omitempty"`
    Output    string          `json:"output,omitempty"`
}

type inputContent struct {
    Type string `json:"type"`
    Text string `json:"text,omitempty"`
    // image variants omitted for v1
}

type payloadTool struct {
    Type        string          `json:"type"`
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func buildPayload(req ai.Request) ([]byte, error) {
    p := payload{
        Model:        req.Model,
        Stream:       true,
        Instructions: req.System,
        ToolChoice:   "auto",
        Store:        false,
    }
    if req.Reasoning.Effort != "" {
        p.Reasoning = map[string]any{"effort": req.Reasoning.Effort, "summary": "auto"}
    }
    for _, t := range req.Tools {
        p.Tools = append(p.Tools, payloadTool{
            Type:        "function",
            Name:        t.Name,
            Description: t.Description,
            Parameters:  t.Schema,
        })
    }
    for _, m := range req.Messages {
        items, err := messageToInputItems(m)
        if err != nil { return nil, err }
        p.Input = append(p.Input, items...)
    }
    return json.Marshal(p)
}

func messageToInputItems(m ai.Message) ([]inputItem, error) {
    switch m.Role {
    case ai.RoleUser, ai.RoleAssistant:
        item := inputItem{
            Type: "message",
            Role: string(m.Role),
        }
        for _, c := range m.Content {
            switch v := c.(type) {
            case ai.TextBlock:
                item.Content = append(item.Content, inputContent{Type: textTypeFor(m.Role), Text: v.Text})
            case ai.ToolUseBlock:
                // Function call items live at the top level, not inside a message.
                // Emit message first (if it has any content), then the call.
                args := string(v.Args)
                if args == "" { args = "{}" }
                items := []inputItem{}
                if len(item.Content) > 0 {
                    items = append(items, item)
                }
                items = append(items, inputItem{
                    Type: "function_call",
                    CallID: v.ID,
                    Name: v.Name,
                    Arguments: args,
                })
                return items, nil
            }
        }
        return []inputItem{item}, nil

    case ai.RoleToolResult:
        for _, c := range m.Content {
            if v, ok := c.(ai.ToolResultBlock); ok {
                return []inputItem{{
                    Type: "function_call_output",
                    CallID: v.ToolCallID,
                    Output: v.Output,
                }}, nil
            }
        }
    }
    return nil, nil
}

func textTypeFor(role ai.Role) string {
    if role == ai.RoleAssistant {
        return "output_text"
    }
    return "input_text"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/codex/ -run TestBuildPayload`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/codex/request.go internal/ai/codex/request_test.go
git commit -m "feat(codex): build Responses API payload from ai.Request"
```

---

## Task 7: Codex provider — Stream() with stub HTTP server

**Files:**
- Create: `internal/ai/codex/codex.go`
- Create: `internal/ai/codex/codex_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ai/codex/codex_test.go
package codex

import (
    "context"
    "io"
    "net/http"
    "net/http/httptest"
    "os"
    "strings"
    "testing"
    "time"

    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/ai/oauth"
)

type stubAuth struct{ tok oauth.Credentials }

func (s *stubAuth) Token(ctx context.Context) (string, error) { return s.tok.AccessToken, nil }
func (s *stubAuth) Refresh(ctx context.Context) error          { return nil }

func TestStream_TextResponse(t *testing.T) {
    fixture, _ := os.ReadFile("testdata/stream_text_only.sse")
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if got := r.Header.Get("Authorization"); got != "Bearer AT" {
            t.Fatalf("auth = %q", got)
        }
        w.Header().Set("Content-Type", "text/event-stream")
        w.WriteHeader(200)
        _, _ = io.Copy(w, strings.NewReader(string(fixture)))
    }))
    defer srv.Close()

    p := New(srv.URL+"/codex/responses", &stubAuth{tok: oauth.Credentials{AccessToken: "AT"}})
    ch, err := p.Stream(context.Background(), ai.Request{
        Model:    "gpt-5",
        Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}}},
    })
    if err != nil { t.Fatal(err) }

    deadline := time.After(2 * time.Second)
    var sb strings.Builder
    var sawDone bool
loop:
    for {
        select {
        case e, ok := <-ch:
            if !ok { break loop }
            switch v := e.(type) {
            case ai.TextDelta:
                sb.WriteString(v.Text)
            case ai.Done:
                sawDone = true
            }
        case <-deadline:
            t.Fatal("stream did not finish")
        }
    }
    if sb.String() != "hello world" {
        t.Fatalf("text = %q", sb.String())
    }
    if !sawDone { t.Fatal("missing Done") }
}

func TestStream_NonStreamingErrorIsRetryable(t *testing.T) {
    var hits int
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        hits++
        if hits < 3 {
            w.WriteHeader(http.StatusTooManyRequests)
            _, _ = w.Write([]byte("rate limited"))
            return
        }
        w.Header().Set("Content-Type", "text/event-stream")
        _, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{}}}\n\n"))
    }))
    defer srv.Close()

    p := New(srv.URL+"/codex/responses", &stubAuth{tok: oauth.Credentials{AccessToken: "AT"}})
    p.retryBaseDelay = 1 * time.Millisecond

    ch, err := p.Stream(context.Background(), ai.Request{Model: "gpt-5"})
    if err != nil { t.Fatal(err) }
    for range ch {
    }
    if hits != 3 {
        t.Fatalf("expected 3 hits after retries, got %d", hits)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/codex/ -run TestStream`
Expected: FAIL — `New` undefined.

- [ ] **Step 3: Implement**

```go
// internal/ai/codex/codex.go
package codex

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "net/http"
    "time"

    "github.com/khang859/rune/internal/ai"
)

type AuthSource interface {
    Token(ctx context.Context) (string, error)
    Refresh(ctx context.Context) error
}

type Provider struct {
    endpoint       string
    auth           AuthSource
    httpClient     *http.Client
    maxRetries     int
    retryBaseDelay time.Duration
}

func New(endpoint string, auth AuthSource) *Provider {
    return &Provider{
        endpoint:       endpoint,
        auth:           auth,
        httpClient:     &http.Client{Timeout: 0}, // streaming, no overall timeout
        maxRetries:     3,
        retryBaseDelay: 1 * time.Second,
    }
}

func (p *Provider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
    body, err := buildPayload(req)
    if err != nil {
        return nil, err
    }
    out := make(chan ai.Event, 64)
    go func() {
        defer close(out)
        if err := p.streamWithRetry(ctx, body, out); err != nil {
            select {
            case out <- ai.StreamError{Err: err, Retryable: false}:
            case <-ctx.Done():
            }
        }
    }()
    return out, nil
}

func (p *Provider) streamWithRetry(ctx context.Context, body []byte, out chan<- ai.Event) error {
    var lastErr error
    for attempt := 0; attempt <= p.maxRetries; attempt++ {
        if ctx.Err() != nil {
            return ctx.Err()
        }
        err := p.streamOnce(ctx, body, out)
        if err == nil {
            return nil
        }
        lastErr = err
        if !isRetryable(err) {
            return err
        }
        wait := p.retryBaseDelay * (1 << attempt)
        select {
        case <-time.After(wait):
        case <-ctx.Done():
            return ctx.Err()
        }
    }
    return lastErr
}

type retryableErr struct{ err error }

func (e retryableErr) Error() string { return e.err.Error() }
func (e retryableErr) Unwrap() error { return e.err }

func isRetryable(err error) bool {
    _, ok := err.(retryableErr)
    return ok
}

func (p *Provider) streamOnce(ctx context.Context, body []byte, out chan<- ai.Event) error {
    token, err := p.auth.Token(ctx)
    if err != nil {
        return err
    }
    httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
    if err != nil { return err }
    httpReq.Header.Set("Authorization", "Bearer "+token)
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Accept", "text/event-stream")
    httpReq.Header.Set("OpenAI-Beta", "responses=v1")

    resp, err := p.httpClient.Do(httpReq)
    if err != nil {
        return retryableErr{err}
    }
    defer resp.Body.Close()

    if resp.StatusCode == http.StatusUnauthorized {
        if rerr := p.auth.Refresh(ctx); rerr != nil {
            return fmt.Errorf("auth refresh failed: %w", rerr)
        }
        return retryableErr{fmt.Errorf("401 unauthorized, refreshed and retrying")}
    }
    if resp.StatusCode == 429 || resp.StatusCode >= 500 {
        b, _ := io.ReadAll(resp.Body)
        return retryableErr{fmt.Errorf("status %d: %s", resp.StatusCode, string(b))}
    }
    if resp.StatusCode >= 400 {
        b, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
    }
    return parseSSE(ctx, resp.Body, out)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/codex/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/codex/codex.go internal/ai/codex/codex_test.go
git commit -m "feat(codex): provider with retry on 429/5xx and 401-refresh"
```

---

## Task 8: Auth source: storage-backed token with refresh

**Files:**
- Create: `internal/ai/oauth/source.go`
- Create: `internal/ai/oauth/source_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ai/oauth/source_test.go
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
        ExpiresAt:    time.Now().Add(-1 * time.Minute), // already expired
    })

    src := &CodexSource{Store: store, TokenURL: srv.URL + "/oauth/token"}
    tok, err := src.Token(context.Background())
    if err != nil { t.Fatal(err) }
    if tok != "AT-NEW" {
        t.Fatalf("token = %q", tok)
    }
    if !refreshed {
        t.Fatal("refresh did not occur")
    }
    // Also persisted.
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/oauth/ -run TestSource`
Expected: FAIL — CodexSource undefined.

- [ ] **Step 3: Implement**

```go
// internal/ai/oauth/source.go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/oauth/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/oauth/source.go internal/ai/oauth/source_test.go
git commit -m "feat(oauth): codex source with auto-refresh on near-expiry"
```

---

## Task 9: `rune login codex` CLI command

**Files:**
- Create: `cmd/rune/login.go`
- Modify: `cmd/rune/main.go`

> Manual verification only — interactive browser flow. We test the wiring by injecting a fake OpenBrowser and stubbing the token URL via `RUNE_OAUTH_TOKEN_URL` env var. CI doesn't exercise the real flow.

- [ ] **Step 1: Implement the login command**

```go
// cmd/rune/login.go
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
    if provider != "codex" {
        return fmt.Errorf("only 'codex' is supported in v1; got %q", provider)
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
```

- [ ] **Step 2: Wire into main**

```go
// cmd/rune/main.go (replace)
package main

import (
    "context"
    "flag"
    "fmt"
    "os"

    "github.com/khang859/rune/internal/ai/faux"
)

const version = "0.0.0-dev"

func main() {
    flag.Usage = func() {
        fmt.Fprintln(os.Stderr, "usage: rune [--script <file>] [--prompt <text>] | rune login codex")
        flag.PrintDefaults()
    }
    script := flag.String("script", "", "run a JSON script (headless smoke runner)")
    prompt := flag.String("prompt", "", "run a single turn against the configured provider and exit")
    flag.Parse()

    ctx := context.Background()

    args := flag.Args()
    if len(args) >= 2 && args[0] == "login" {
        if err := runLogin(ctx, args[1]); err != nil {
            fmt.Fprintln(os.Stderr, "login error:", err)
            os.Exit(1)
        }
        return
    }
    if *script != "" {
        if err := runScript(ctx, *script, os.Stdout, faux.New()); err != nil {
            fmt.Fprintln(os.Stderr, "error:", err)
            os.Exit(1)
        }
        return
    }
    if *prompt != "" {
        if err := runPrompt(ctx, *prompt, os.Stdout); err != nil {
            fmt.Fprintln(os.Stderr, "error:", err)
            os.Exit(1)
        }
        return
    }
    fmt.Println("rune", version)
}
```

- [ ] **Step 3: Build to confirm it compiles (runPrompt declared next task)**

We will fail the build until Task 10 lands `runPrompt`. Stub it now to keep the build green.

```go
// cmd/rune/prompt.go (stub for now — full impl in Task 10)
package main

import (
    "context"
    "errors"
    "io"
)

func runPrompt(ctx context.Context, text string, w io.Writer) error {
    return errors.New("--prompt not yet implemented — Task 10")
}
```

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 4: Commit**

```bash
git add cmd/rune/login.go cmd/rune/main.go cmd/rune/prompt.go
git commit -m "feat(cmd): rune login codex command (manual flow only)"
```

---

## Task 10: `rune --prompt "..."` one-shot mode against real Codex

**Files:**
- Replace: `cmd/rune/prompt.go`
- Create: `cmd/rune/prompt_test.go`

- [ ] **Step 1: Write the failing test**

```go
// cmd/rune/prompt_test.go
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

    // Pre-seed credentials.
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

    // Session was written.
    sessions, _ := os.ReadDir(filepath.Join(runeDir, "sessions"))
    if len(sessions) == 0 {
        t.Fatal("no session file written")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/rune/ -run TestRunPrompt`
Expected: FAIL — runPrompt is the stub from Task 9.

- [ ] **Step 3: Implement**

```go
// cmd/rune/prompt.go (replace)
package main

import (
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"

    "github.com/khang859/rune/internal/agent"
    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/ai/codex"
    "github.com/khang859/rune/internal/ai/oauth"
    "github.com/khang859/rune/internal/config"
    "github.com/khang859/rune/internal/session"
    "github.com/khang859/rune/internal/tools"
)

func runPrompt(ctx context.Context, text string, w io.Writer) error {
    if err := config.EnsureRuneDir(); err != nil {
        return err
    }
    endpoint := oauth.CodexResponsesBaseURL + oauth.CodexResponsesPath
    if v := os.Getenv("RUNE_CODEX_ENDPOINT"); v != "" {
        endpoint = v
    }
    tokenURL := oauth.CodexTokenURL
    if v := os.Getenv("RUNE_OAUTH_TOKEN_URL"); v != "" {
        tokenURL = v
    }

    store := oauth.NewStore(config.AuthPath())
    src := &oauth.CodexSource{Store: store, TokenURL: tokenURL}
    if _, err := src.Token(ctx); err != nil {
        return fmt.Errorf("not logged in: %w (run `rune login codex`)", err)
    }

    p := codex.New(endpoint, src)
    sess := session.New("gpt-5")
    sess.SetPath(filepath.Join(config.SessionsDir(), sess.ID+".json"))

    reg := tools.NewRegistry()
    reg.Register(tools.Read{})
    reg.Register(tools.Write{})
    reg.Register(tools.Edit{})
    reg.Register(tools.Bash{})

    a := agent.New(p, reg, sess, defaultSystemPrompt())
    msg := ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
    for ev := range a.Run(ctx, msg) {
        switch v := ev.(type) {
        case agent.AssistantText:
            fmt.Fprint(w, v.Delta)
        case agent.ToolStarted:
            fmt.Fprintf(w, "\n[tool: %s]", v.Call.Name)
        case agent.ToolFinished:
            fmt.Fprintf(w, "\n[done: %d bytes]", len(v.Result.Output))
        case agent.TurnError:
            fmt.Fprintf(w, "\n[error: %v]", v.Err)
        }
    }
    fmt.Fprintln(w)
    return sess.Save()
}

func defaultSystemPrompt() string {
    // AGENTS.md hierarchy walking lands later — keep minimal for now.
    return "You are rune, a coding agent. Use the available tools."
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/rune/...`
Expected: PASS

- [ ] **Step 5: Manual smoke test (optional, requires real login)**

```bash
go run ./cmd/rune login codex      # browser opens
go run ./cmd/rune --prompt "say hi"
```
Expected: streams a response from real Codex.

- [ ] **Step 6: Commit**

```bash
git add cmd/rune/prompt.go cmd/rune/prompt_test.go
git commit -m "feat(cmd): --prompt one-shot mode against real Codex"
```

---

## Task 11: AGENTS.md context loader

**Files:**
- Create: `internal/agent/context.go`
- Create: `internal/agent/context_test.go`
- Modify: `cmd/rune/prompt.go` to use it

- [ ] **Step 1: Write the failing test**

```go
// internal/agent/context_test.go
package agent

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestLoadAgentsMD_WalksUp(t *testing.T) {
    base := t.TempDir()
    inner := filepath.Join(base, "a", "b", "c")
    _ = os.MkdirAll(inner, 0o755)
    _ = os.WriteFile(filepath.Join(base, "AGENTS.md"), []byte("ROOT"), 0o644)
    _ = os.WriteFile(filepath.Join(base, "a", "AGENTS.md"), []byte("MID"), 0o644)
    _ = os.WriteFile(filepath.Join(inner, "AGENTS.md"), []byte("LEAF"), 0o644)

    got := LoadAgentsMD(inner, base) // stop walking at base

    if !strings.Contains(got, "ROOT") || !strings.Contains(got, "MID") || !strings.Contains(got, "LEAF") {
        t.Fatalf("missing layers: %q", got)
    }
    // Root must come first (broadest scope), leaf last (most specific).
    rootIdx := strings.Index(got, "ROOT")
    leafIdx := strings.Index(got, "LEAF")
    if rootIdx > leafIdx {
        t.Fatalf("ordering wrong: root=%d leaf=%d", rootIdx, leafIdx)
    }
}

func TestLoadAgentsMD_NoneFound(t *testing.T) {
    dir := t.TempDir()
    if got := LoadAgentsMD(dir, dir); got != "" {
        t.Fatalf("expected empty, got %q", got)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestLoadAgentsMD`
Expected: FAIL — `LoadAgentsMD` undefined.

- [ ] **Step 3: Implement**

```go
// internal/agent/context.go
package agent

import (
    "os"
    "path/filepath"
    "strings"
)

// LoadAgentsMD walks from start up to (and including) stop, collecting AGENTS.md files.
// Returns them concatenated, root-first (broadest scope first).
func LoadAgentsMD(start, stop string) string {
    var collected []string
    cur, _ := filepath.Abs(start)
    stopAbs, _ := filepath.Abs(stop)
    for {
        p := filepath.Join(cur, "AGENTS.md")
        if b, err := os.ReadFile(p); err == nil {
            collected = append([]string{string(b)}, collected...) // prepend
        }
        if cur == stopAbs {
            break
        }
        parent := filepath.Dir(cur)
        if parent == cur {
            break
        }
        cur = parent
    }
    return strings.Join(collected, "\n\n---\n\n")
}
```

- [ ] **Step 4: Wire into prompt.go**

```go
// cmd/rune/prompt.go — replace defaultSystemPrompt usage
import "github.com/khang859/rune/internal/agent" // already imported

cwd, _ := os.Getwd()
home, _ := os.UserHomeDir()
agentsMD := agent.LoadAgentsMD(cwd, home)

system := defaultSystemPrompt()
if agentsMD != "" {
    system += "\n\nProject context:\n" + agentsMD
}
a := agent.New(p, reg, sess, system)
```

(Replace the corresponding block in `runPrompt`.)

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/agent/context.go internal/agent/context_test.go cmd/rune/prompt.go
git commit -m "feat(agent): walk AGENTS.md up to home for system prompt"
```

---

## End state

After Plan 02, rune has:

- `rune login codex` — real PKCE flow, persists `~/.rune/auth.json` under file lock.
- `rune --prompt "..."` — one-shot turn against real Codex via the same agent loop from Plan 01, with auto-refresh, retry on 429/5xx, AGENTS.md context loading.
- Full SSE parser exercised against captured fixtures.
- Token store handles concurrent rune processes correctly.

Plan 03 introduces the Bubble Tea TUI on top of this.
