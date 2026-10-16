package domain_test

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// RFC 6238's SHA-1 test secret, "12345678901234567890" in base32.
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" //nolint:gosec // RFC 6238's published test secret gitleaks:allow

func TestATOTPCodeMatchesItsStepAndTheNeighboursOnce(t *testing.T) {
	// RFC 6238 appendix B: at 59s the 8-digit code is 94287082, so the 6-digit one is 287082.
	at := time.Unix(59, 0)
	if step, ok := domain.MatchTOTP(rfcSecret, "287082", at, 0); !ok || step != 1 {
		t.Fatalf("the RFC vector = %d, %v", step, ok)
	}
	if _, ok := domain.MatchTOTP(rfcSecret, "287082", at, 1); ok {
		t.Fatal("a code was accepted twice")
	}
	now := time.Date(2026, 10, 16, 12, 0, 15, 0, time.UTC)
	for _, offset := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
		code, _ := totp.GenerateCode(rfcSecret, now.Add(offset))
		if _, ok := domain.MatchTOTP(rfcSecret, code, now, 0); !ok {
			t.Errorf("a code %v away was refused", offset)
		}
	}
	for _, offset := range []time.Duration{-60 * time.Second, 60 * time.Second} {
		code, _ := totp.GenerateCode(rfcSecret, now.Add(offset))
		if _, ok := domain.MatchTOTP(rfcSecret, code, now, 0); ok {
			t.Errorf("a code %v away was accepted", offset)
		}
	}
	for _, bad := range []string{"", "12345", "1234567", "12a456", " 287082"} {
		if _, ok := domain.MatchTOTP(rfcSecret, bad, at, 0); ok {
			t.Errorf("%q was accepted", bad)
		}
	}
	if _, ok := domain.MatchTOTP("not base32!", "287082", at, 0); ok {
		t.Error("a broken secret matched")
	}
}

func TestANewTOTPSecretAndItsURI(t *testing.T) {
	secret, err := domain.NewTOTPSecret(bytes.NewReader(bytes.Repeat([]byte{0xff}, 20)))
	if err != nil || len(secret) != 32 || strings.Contains(secret, "=") {
		t.Fatalf("NewTOTPSecret = %q, %v", secret, err)
	}
	if _, err := domain.NewTOTPSecret(bytes.NewReader(nil)); err == nil {
		t.Fatal("no randomness made a secret")
	}
	u, err := url.Parse(domain.TOTPURI(secret, "ada@example.org"))
	if err != nil || u.Scheme != "otpauth" || u.Host != "totp" || u.Path != "/jorisjonkers.dev:ada@example.org" {
		t.Fatalf("URI = %v, %v", u, err)
	}
	q := u.Query()
	if q.Get("secret") != secret || q.Get("issuer") != "jorisjonkers.dev" || q.Get("digits") != "6" || q.Get("period") != "30" || q.Get("algorithm") != "SHA1" {
		t.Fatalf("URI query = %v", q)
	}
}

func TestAFactorLabel(t *testing.T) {
	if l, err := domain.NewFactorLabel("  Phone "); err != nil || l.String() != "Phone" {
		t.Fatalf("NewFactorLabel = %q, %v", l, err)
	}
	for _, bad := range []string{" ", strings.Repeat("x", 65)} {
		if _, err := domain.NewFactorLabel(bad); !errors.Is(err, domain.ErrInvalidLabel) {
			t.Errorf("NewFactorLabel(%q) = %v", bad, err)
		}
	}
}
