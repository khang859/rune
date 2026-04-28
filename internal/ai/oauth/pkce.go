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
