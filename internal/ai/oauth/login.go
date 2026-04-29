package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type LoginConfig struct {
	AuthorizeURL string
	TokenURL     string
	Port         int
	OpenBrowser  func(authURL string) error
}

type LoginFlow struct {
	cfg    LoginConfig
	pkce   PKCE
	state  string
	srv    *http.Server
	listen net.Listener
	once   sync.Once
	result chan loginResult
	cbPath string
}

type loginResult struct {
	creds Credentials
	err   error
}

func StartLogin(cfg LoginConfig) (*LoginFlow, error) {
	pkce, err := GeneratePKCE()
	if err != nil {
		return nil, err
	}
	state, err := GenerateState()
	if err != nil {
		return nil, err
	}

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

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
		_ = cfg.OpenBrowser(cfg.AuthorizeURL)
	}
	return flow, nil
}

func (f *LoginFlow) State() string     { return f.state }
func (f *LoginFlow) Verifier() string  { return f.pkce.Verifier }
func (f *LoginFlow) Challenge() string { return f.pkce.Challenge }
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
