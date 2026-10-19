package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

const fresh = "a brand new passphrase"

// resetToken is the token in the last mail's reset link.
func (f *fixture) resetToken(t *testing.T) string {
	t.Helper()
	text := f.mailer.sent[len(f.mailer.sent)-1].Text
	_, after, ok := strings.Cut(text, "https://auth.test/reset?token=")
	if !ok {
		t.Fatalf("no reset link in %q", text)
	}
	return strings.Fields(after)[0]
}

// fromClient is a context naming the client a request came from, as the web adapter names it.
func fromClient(address string) context.Context {
	return app.WithClient(context.Background(), address)
}

// resetting is Ada, without two-factor, with a reset link mailed to her.
func resetting(t *testing.T) (*fixture, uuid.UUID, string) {
	t.Helper()
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if err := f.AskPasswordReset(context.Background(), "ada@example.org"); err != nil {
		t.Fatal(err)
	}
	return f, id, f.resetToken(t)
}

func TestAResetMailsALinkThatWorksForThirtyMinutes(t *testing.T) {
	f, id, _ := resetting(t)
	m := f.mailer.sent[len(f.mailer.sent)-1]
	if m.To != "ada@example.org" || m.Subject != "Reset your password" || !f.store.resets[id].ExpiresAt.Equal(t0.Add(30*time.Minute)) {
		t.Fatalf("mail %+v, reset %+v", m, f.store.resets[id])
	}
}

func TestAResetForAnUnknownEmailMailsNothing(t *testing.T) {
	f := newFixture()
	if err := f.AskPasswordReset(context.Background(), "nobody@example.org"); err != nil || len(f.mailer.sent) != 0 {
		t.Fatalf("err %v, sent %d", err, len(f.mailer.sent))
	}
}

func TestAResetForSomethingThatIsNotAnEmailIsRefused(t *testing.T) {
	if err := newFixture().AskPasswordReset(context.Background(), "ada"); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("err = %v", err)
	}
}

func TestResetsAreLimitedPerAddressOverAWindow(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	sent := len(f.mailer.sent)
	for range 4 {
		_ = f.AskPasswordReset(context.Background(), "ada@example.org")
	}
	if len(f.mailer.sent)-sent != 3 {
		t.Fatalf("%d mails, want 3", len(f.mailer.sent)-sent)
	}
	f.now = t0.Add(15 * time.Minute)
	_ = f.AskPasswordReset(context.Background(), "ada@example.org")
	if len(f.mailer.sent)-sent != 4 {
		t.Fatalf("%d mails after the window, want 4", len(f.mailer.sent)-sent)
	}
}

func TestResetsAreLimitedPerClient(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	sent := len(f.mailer.sent)
	client := fromClient("198.51.100.7")
	for i := range 10 {
		_ = f.AskPasswordReset(client, "nobody"+string(rune('a'+i))+"@example.org")
	}
	_ = f.AskPasswordReset(client, "ada@example.org")
	_ = f.AskPasswordReset(fromClient("203.0.113.9"), "ada@example.org")
	if len(f.mailer.sent)-sent != 1 {
		t.Fatalf("%d mails, want only the one from another client", len(f.mailer.sent)-sent)
	}
}

func TestAResetLinkChangesThePasswordEndsEverySessionAndMailsTheUser(t *testing.T) {
	f, _, token := resetting(t)
	before, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	if err := f.CompletePasswordReset(context.Background(), token, fresh); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.store.sessions[sum(before)]; ok {
		t.Fatal("a session from before the reset survived")
	}
	if _, err := f.SignIn(context.Background(), "ada@example.org", fresh); err != nil {
		t.Fatalf("the new password = %v", err)
	}
	m := f.mailer.sent[len(f.mailer.sent)-1]
	if m.Subject != "Your password was reset" || len(f.store.resets) != 0 {
		t.Fatalf("mail %+v, resets %v", m, f.store.resets)
	}
}

func TestAResetIsAnAuditEvent(t *testing.T) {
	f, id, token := resetting(t)
	*f.events = nil
	_ = f.CompletePasswordReset(context.Background(), token, fresh)
	if len(*f.events) != 1 || (*f.events)[0] != (audit.Event{Action: audit.PasswordReset, User: id}) {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestAResetLinkFails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before func(f *fixture, token string)
	}{
		{"once spent", func(f *fixture, token string) { _ = f.CompletePasswordReset(context.Background(), token, fresh) }},
		{"after thirty minutes", func(f *fixture, _ string) { f.now = t0.Add(30 * time.Minute) }},
		{"once a newer link replaced it", func(f *fixture, _ string) {
			f.now = t0.Add(time.Minute)
			_ = f.AskPasswordReset(context.Background(), "ada@example.org")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, token := resetting(t)
			tc.before(f, token)
			if err := f.CompletePasswordReset(context.Background(), token, "another passphrase"); !errors.Is(err, app.ErrInvalidToken) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAResetRacedByAnotherIsSpent(t *testing.T) {
	f, _, token := resetting(t)
	// A racing reset takes the link between reading it and spending it.
	f.store.racedReset = true
	if err := f.CompletePasswordReset(context.Background(), token, fresh); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestANewPasswordMustBeLongEnough(t *testing.T) {
	f, _, token := resetting(t)
	if err := f.CompletePasswordReset(context.Background(), token, "short"); !errors.Is(err, domain.ErrWeakPassword) {
		t.Fatalf("err = %v", err)
	}
}

func TestAUserWithoutTwoFactorHasNothingToProve(t *testing.T) {
	f, _, token := resetting(t)
	if kinds, err := f.CheckPasswordReset(context.Background(), token); len(kinds) != 0 || err != nil {
		t.Fatalf("CheckPasswordReset = %v, %v", kinds, err)
	}
}

// resettingWithTwoFactor is Ada, with an authenticator, a minute after enrolling, with a reset
// link mailed to her and checked.
func resettingWithTwoFactor(t *testing.T) (f *fixture, secret, token string) {
	t.Helper()
	f = newFixture()
	_, secret, _ = f.enrolled(t, "ada@example.org")
	f.now = t0.Add(time.Minute)
	_ = f.AskPasswordReset(context.Background(), "ada@example.org")
	token = f.resetToken(t)
	if kinds, err := f.CheckPasswordReset(context.Background(), token); err != nil || len(kinds) != 1 || kinds[0] != app.KindTOTP {
		t.Fatalf("CheckPasswordReset = %v, %v", kinds, err)
	}
	return f, secret, token
}

func TestAUserWithTwoFactorProvesItBeforeTheNewPassword(t *testing.T) {
	f, secret, token := resettingWithTwoFactor(t)
	if err := f.CompletePasswordReset(context.Background(), token, fresh); !errors.Is(err, app.ErrFreshTwoFactorNeeded) {
		t.Fatalf("before = %v", err)
	}
	if err := f.ResetWithCode(context.Background(), token, code(t, secret, f.now)); err != nil {
		t.Fatal(err)
	}
	if err := f.CompletePasswordReset(context.Background(), token, fresh); err != nil {
		t.Fatalf("after = %v", err)
	}
}

func TestAProvedResetHasNothingMoreToProve(t *testing.T) {
	f, secret, token := resettingWithTwoFactor(t)
	_ = f.ResetWithCode(context.Background(), token, code(t, secret, f.now))
	if kinds, err := f.CheckPasswordReset(context.Background(), token); len(kinds) != 0 || err != nil {
		t.Fatalf("CheckPasswordReset = %v, %v", kinds, err)
	}
}

func TestAWrongResetCodeIsRefused(t *testing.T) {
	f, _, token := resettingWithTwoFactor(t)
	if err := f.ResetWithCode(context.Background(), token, "000000"); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("err = %v", err)
	}
}

func TestAPasskeyProvesAReset(t *testing.T) {
	f := newFixture()
	id, _ := f.withPasskey(t)
	f.now = t0.Add(time.Minute)
	_ = f.AskPasswordReset(context.Background(), "ada@example.org")
	token := f.resetToken(t)
	ctx := context.Background()
	if kinds, _ := f.CheckPasswordReset(ctx, token); len(kinds) != 1 || kinds[0] != app.KindPasskey {
		t.Fatalf("kinds = %v", kinds)
	}
	if _, err := f.ResetPasskeyOptions(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := f.ResetWithPasskey(ctx, token, []byte("signed:login-"+id.String()+"|cred-1")); err != nil {
		t.Fatal(err)
	}
	if err := f.CompletePasswordReset(ctx, token, fresh); err != nil {
		t.Fatalf("after = %v", err)
	}
}

func TestARefusedResetPasskeyIsRefused(t *testing.T) {
	f := newFixture()
	f.withPasskey(t)
	_ = f.AskPasswordReset(context.Background(), "ada@example.org")
	token := f.resetToken(t)
	ctx := context.Background()
	_, _ = f.CheckPasswordReset(ctx, token)
	_, _ = f.ResetPasskeyOptions(ctx, token)
	if err := f.ResetWithPasskey(ctx, token, []byte("signed:other|cred-1")); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("err = %v", err)
	}
}

func TestEveryResetStepRefusesASpentLink(t *testing.T) {
	for _, step := range []struct {
		name string
		call func(f *fixture) error
	}{
		{"checking", func(f *fixture) error { _, err := f.CheckPasswordReset(context.Background(), "spent"); return err }},
		{"a code", func(f *fixture) error { return f.ResetWithCode(context.Background(), "spent", "123456") }},
		{"passkey options", func(f *fixture) error { _, err := f.ResetPasskeyOptions(context.Background(), "spent"); return err }},
		{"a passkey", func(f *fixture) error { return f.ResetWithPasskey(context.Background(), "spent", nil) }},
	} {
		t.Run(step.name, func(t *testing.T) {
			if err := step.call(newFixture()); !errors.Is(err, app.ErrInvalidToken) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAPasskeyResetWithoutCheckingFirstIsRefused(t *testing.T) {
	f := newFixture()
	id, _ := f.withPasskey(t)
	_ = f.AskPasswordReset(context.Background(), "ada@example.org")
	token := f.resetToken(t)
	if err := f.ResetWithPasskey(context.Background(), token, []byte("signed:login-"+id.String()+"|cred-1")); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestAResetFailsWhenTheStoreDoes(t *testing.T) {
	for _, tc := range []struct {
		method string
		call   func(f *fixture, token, c string) error
	}{
		{"UserByEmail", func(f *fixture, _, _ string) error {
			return f.AskPasswordReset(context.Background(), "ada@example.org")
		}},
		{"SavePasswordReset", func(f *fixture, _, _ string) error {
			f.now = f.now.Add(time.Hour)
			return f.AskPasswordReset(context.Background(), "ada@example.org")
		}},
		{"PasswordResetByHash", func(f *fixture, token, _ string) error {
			_, err := f.CheckPasswordReset(context.Background(), token)
			return err
		}},
		{"Passkeys", func(f *fixture, token, _ string) error {
			_, err := f.CheckPasswordReset(context.Background(), token)
			return err
		}},
		{"SaveChallenge", func(f *fixture, token, _ string) error {
			_, err := f.CheckPasswordReset(context.Background(), token)
			return err
		}},
		{"ProvePasswordReset", func(f *fixture, token, c string) error { return f.ResetWithCode(context.Background(), token, c) }},
		{"TakePasswordReset", func(f *fixture, token, c string) error {
			_ = f.ResetWithCode(context.Background(), token, c)
			return f.CompletePasswordReset(context.Background(), token, fresh)
		}},
		{"UpdatePasswordHash", func(f *fixture, token, c string) error {
			_ = f.ResetWithCode(context.Background(), token, c)
			return f.CompletePasswordReset(context.Background(), token, fresh)
		}},
		{"DeleteOtherSessions", func(f *fixture, token, c string) error {
			_ = f.ResetWithCode(context.Background(), token, c)
			return f.CompletePasswordReset(context.Background(), token, fresh)
		}},
		{"UserByID", func(f *fixture, token, c string) error {
			_ = f.ResetWithCode(context.Background(), token, c)
			return f.CompletePasswordReset(context.Background(), token, fresh)
		}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			f := newFixture()
			_, secret, _ := f.enrolled(t, "ada@example.org")
			f.now = t0.Add(time.Minute)
			_ = f.AskPasswordReset(context.Background(), "ada@example.org")
			token := f.resetToken(t)
			if tc.method != "PasswordResetByHash" && tc.method != "Passkeys" && tc.method != "SaveChallenge" {
				_, _ = f.CheckPasswordReset(context.Background(), token)
			}
			c := code(t, secret, f.now)
			f.store.fail[tc.method] = errBoomSF
			if err := tc.call(f, token, c); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestNoResetIsMadeWithoutRandomness(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	f.dry = true
	if err := f.AskPasswordReset(context.Background(), "ada@example.org"); err == nil {
		t.Fatal("a reset was made without randomness")
	}
}
