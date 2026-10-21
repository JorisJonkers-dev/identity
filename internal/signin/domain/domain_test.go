package domain_test

import (
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

func TestOnlyAPKCEClientIsPublic(t *testing.T) {
	if !(domain.Client{Credential: domain.PKCE}).Public() || (domain.Client{Credential: domain.Secret}).Public() || (domain.Client{}).Public() {
		t.Fatal("Public reads the credential wrong")
	}
}

func TestAnAskedProofIsTheMaxAgeTheAppNamed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		maxAge *uint
		want   time.Duration
	}{
		{"five minutes", ptr(300), 5 * time.Minute},
		{"no max_age, an hour", nil, time.Hour},
		{"under a minute, a minute", ptr(0), time.Minute},
		{"over an hour, an hour", ptr(7200), time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.AskedProof(tc.maxAge); got != tc.want {
				t.Fatalf("AskedProof = %v, want %v", got, tc.want)
			}
		})
	}
}

func ptr(n uint) *uint { return &n }

func TestTheLargestMaxAgeAsksForAProofAtMostAnHourOld(t *testing.T) {
	if got := domain.AskedProof(ptr(^uint(0))); got != time.Hour {
		t.Fatalf("AskedProof = %v", got)
	}
}
