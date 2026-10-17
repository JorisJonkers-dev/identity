package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/tokens/domain"
)

var t0 = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

func TestATokenLastsThirtyNinetyOrThreeHundredSixtyFiveDays(t *testing.T) {
	t.Parallel()
	for _, days := range []int{30, 90, 365} {
		l, err := domain.NewLifetime(days)
		if err != nil || l.From(t0).Sub(t0) != time.Duration(days)*24*time.Hour {
			t.Fatalf("%d days: %v, %v", days, l.From(t0), err)
		}
	}
}

func TestAnyOtherLifetimeIsRefused(t *testing.T) {
	t.Parallel()
	for _, days := range []int{0, 7, 31, 366, -30} {
		if _, err := domain.NewLifetime(days); !errors.Is(err, domain.ErrInvalidLifetime) {
			t.Fatalf("%d days: %v", days, err)
		}
	}
}

func TestATokenNameIsTrimmedAndKept(t *testing.T) {
	t.Parallel()
	if n, err := domain.NewName("  backup-script "); err != nil || n.String() != "backup-script" {
		t.Fatalf("name = %q, %v", n, err)
	}
}

func TestATokenNameOfSixtyFourCharactersIsKept(t *testing.T) {
	t.Parallel()
	if _, err := domain.NewName(strings.Repeat("é", 64)); err != nil {
		t.Fatal(err)
	}
}

func TestAnEmptyOrTooLongTokenNameIsRefused(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{"empty": "", "spaces": "   ", "65 characters": strings.Repeat("a", 65)} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := domain.NewName(raw); !errors.Is(err, domain.ErrInvalidName) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestATokenIsLiveUntilItExpiresOrIsRevoked(t *testing.T) {
	t.Parallel()
	revoked := t0
	for _, tc := range []struct {
		name  string
		token domain.Token
		at    time.Time
		live  bool
	}{
		{"before it expires", domain.Token{ExpiresAt: t0.Add(time.Hour)}, t0, true},
		{"when it expires", domain.Token{ExpiresAt: t0}, t0, false},
		{"once revoked", domain.Token{ExpiresAt: t0.Add(time.Hour), RevokedAt: &revoked}, t0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.token.Live(tc.at); got != tc.live {
				t.Fatalf("live = %v", got)
			}
		})
	}
}

func TestATokenStandsAsRevokedExpiredExpiringOrActive(t *testing.T) {
	t.Parallel()
	revoked := t0
	for _, tc := range []struct {
		name  string
		token domain.Token
		want  domain.State
	}{
		{"revoked, even before it expires", domain.Token{ExpiresAt: t0.AddDate(0, 0, 30), RevokedAt: &revoked}, domain.Revoked},
		{"at its expiry", domain.Token{ExpiresAt: t0}, domain.Expired},
		{"seven days from its end", domain.Token{ExpiresAt: t0.Add(domain.Soon)}, domain.Expiring},
		{"a moment more than seven days from its end", domain.Token{ExpiresAt: t0.Add(domain.Soon + time.Second)}, domain.Active},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.token.State(t0); got != tc.want {
				t.Fatalf("state = %s, want %s", got, tc.want)
			}
		})
	}
}
