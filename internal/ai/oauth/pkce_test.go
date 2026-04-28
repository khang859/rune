package oauth

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestGeneratePKCE_VerifierIsUniqueAndUrlSafe(t *testing.T) {
	a, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
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
