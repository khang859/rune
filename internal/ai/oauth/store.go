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

type lockedFile struct {
	path string
	f    *os.File
}

func (s *Store) openLocked(mode int) (*lockedFile, error) {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, err
	}
	_ = os.Chmod(filepath.Dir(s.path), 0o700)
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
