package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

// steppingUp is Ada with an authenticator, ten minutes after she proved it, with step-up started.
func steppingUp(t *testing.T) (*fixture, string, string) {
	t.Helper()
	f := newFixture()
	_, secret, session := f.enrolled(t, "ada@example.org")
	f.now = t0.Add(10 * time.Minute)
	if _, err := f.StartStepUp(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	return f, secret, session
}

func TestStartingStepUpSaysWhichKindsOfSecondFactorTheUserHas(t *testing.T) {
	f := newFixture()
	_, _, session := f.enrolled(t, "ada@example.org")
	if kinds, err := f.StartStepUp(context.Background(), session); err != nil || !slices.Equal(kinds, []string{app.KindTOTP}) {
		t.Fatalf("StartStepUp = %v, %v", kinds, err)
	}
}

func TestStartingStepUpWithNoSecondFactorIsRefused(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	if _, err := f.StartStepUp(context.Background(), session); !errors.Is(err, app.ErrNoSecondFactor) {
		t.Fatalf("err = %v", err)
	}
}

func TestARightStepUpCodeProvesTheSessionNow(t *testing.T) {
	f, secret, session := steppingUp(t)
	if err := f.StepUpWithCode(context.Background(), session, code(t, secret, f.now)); err != nil {
		t.Fatal(err)
	}
	if s := f.store.sessions[sum(session)]; s.TwoFactorAt == nil || !s.TwoFactorAt.Equal(f.now) || !slices.Equal(s.Methods, []string{"pwd", "otp"}) {
		t.Fatalf("session = %+v", s)
	}
}

func TestAWrongStepUpCodeLeavesTheProofAsItWas(t *testing.T) {
	f, _, session := steppingUp(t)
	if err := f.StepUpWithCode(context.Background(), session, "000000"); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("err = %v", err)
	}
	if s := f.store.sessions[sum(session)]; !s.TwoFactorAt.Equal(t0) {
		t.Fatalf("two-factor at %v", s.TwoFactorAt)
	}
}

func TestAStepUpTakesFiveCodes(t *testing.T) {
	f, secret, session := steppingUp(t)
	for range 5 {
		_ = f.StepUpWithCode(context.Background(), session, "000000")
	}
	if err := f.StepUpWithCode(context.Background(), session, code(t, secret, f.now)); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("a sixth code = %v", err)
	}
}

func TestAStepUpCodeWorksOnce(t *testing.T) {
	f, secret, session := steppingUp(t)
	c := code(t, secret, f.now)
	_ = f.StepUpWithCode(context.Background(), session, c)
	if err := f.StepUpWithCode(context.Background(), session, c); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("the same step-up again = %v", err)
	}
}

func TestAStepUpCodeWithoutStartingStepUpIsRefused(t *testing.T) {
	f := newFixture()
	_, secret, session := f.enrolled(t, "ada@example.org")
	f.now = t0.Add(10 * time.Minute)
	if err := f.StepUpWithCode(context.Background(), session, code(t, secret, f.now)); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestAPasskeyProvesTheSessionNow(t *testing.T) {
	f := newFixture()
	id, session := f.withPasskey(t)
	f.now = t0.Add(10 * time.Minute)
	ctx := context.Background()
	if kinds, err := f.StartStepUp(ctx, session); err != nil || !slices.Equal(kinds, []string{app.KindPasskey}) {
		t.Fatalf("StartStepUp = %v, %v", kinds, err)
	}
	if _, err := f.StepUpPasskeyOptions(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := f.StepUpWithPasskey(ctx, session, []byte("signed:login-"+id.String()+"|cred-1")); err != nil {
		t.Fatal(err)
	}
	if s := f.store.sessions[sum(session)]; s.TwoFactorAt == nil || !s.TwoFactorAt.Equal(f.now) || !slices.Equal(s.Methods, []string{"pwd", "hwk"}) {
		t.Fatalf("session = %+v", s)
	}
}

func TestARefusedStepUpPasskeyLeavesTheProofAsItWas(t *testing.T) {
	f := newFixture()
	_, session := f.withPasskey(t)
	f.now = t0.Add(10 * time.Minute)
	ctx := context.Background()
	_, _ = f.StartStepUp(ctx, session)
	_, _ = f.StepUpPasskeyOptions(ctx, session)
	if err := f.StepUpWithPasskey(ctx, session, []byte("signed:other|cred-1")); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("err = %v", err)
	}
	if s := f.store.sessions[sum(session)]; !s.TwoFactorAt.Equal(t0) {
		t.Fatalf("two-factor at %v", s.TwoFactorAt)
	}
}

func TestStepUpPasskeyOptionsWithoutStartingStepUpAreRefused(t *testing.T) {
	f := newFixture()
	_, session := f.withPasskey(t)
	if _, err := f.StepUpPasskeyOptions(context.Background(), session); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestAStepUpPasskeyWithoutStartingStepUpIsRefused(t *testing.T) {
	f := newFixture()
	id, session := f.withPasskey(t)
	if err := f.StepUpWithPasskey(context.Background(), session, []byte("signed:login-"+id.String()+"|cred-1")); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("err = %v", err)
	}
}

func TestASessionStartedWithASecondFactorSaysWhenItProvedIt(t *testing.T) {
	f, challenge, c := codeStep(t)
	session, err := f.CompleteSignIn(context.Background(), challenge, c)
	if err != nil {
		t.Fatal(err)
	}
	if s := f.store.sessions[sum(session)]; s.TwoFactorAt == nil || !s.TwoFactorAt.Equal(f.now) {
		t.Fatalf("two-factor at %v", s.TwoFactorAt)
	}
}

func TestAPasswordSessionNeverProvedASecondFactor(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	if s := f.store.sessions[sum(session)]; s.TwoFactorAt != nil {
		t.Fatalf("two-factor at %v", s.TwoFactorAt)
	}
}

func TestEnrollingProvesTheSessionNow(t *testing.T) {
	f := newFixture()
	_, _, session := f.enrolled(t, "ada@example.org")
	if s := f.store.sessions[sum(session)]; s.TwoFactorAt == nil || !s.TwoFactorAt.Equal(t0) {
		t.Fatalf("two-factor at %v", s.TwoFactorAt)
	}
}

func TestAUserHasASecondFactorOnceTheyAddOne(t *testing.T) {
	f := newFixture()
	none := f.signedUp(t, "bob@example.org")
	id, _, _ := f.enrolled(t, "ada@example.org")
	if has, err := f.HasSecondFactor(context.Background(), none); has || err != nil {
		t.Fatalf("without one = %v, %v", has, err)
	}
	if has, err := f.HasSecondFactor(context.Background(), id); !has || err != nil {
		t.Fatalf("with one = %v, %v", has, err)
	}
}

func TestStepUpFailsWhenTheStoreDoes(t *testing.T) {
	for _, tc := range []struct {
		method string
		call   func(f *fixture, session, code string) error
	}{
		{"SessionByHash", func(f *fixture, session, _ string) error {
			_, err := f.StartStepUp(context.Background(), session)
			return err
		}},
		{"TOTPFactors", func(f *fixture, session, _ string) error {
			_, err := f.StartStepUp(context.Background(), session)
			return err
		}},
		{"SaveChallenge", func(f *fixture, session, _ string) error {
			_, err := f.StartStepUp(context.Background(), session)
			return err
		}},
		{"StepUpChallenge", func(f *fixture, session, c string) error {
			return f.StepUpWithCode(context.Background(), session, c)
		}},
		{"SetSessionMethods", func(f *fixture, session, c string) error {
			return f.StepUpWithCode(context.Background(), session, c)
		}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			f := newFixture()
			_, secret, session := f.enrolled(t, "ada@example.org")
			f.now = t0.Add(10 * time.Minute)
			if tc.method == "StepUpChallenge" || tc.method == "SetSessionMethods" {
				_, _ = f.StartStepUp(context.Background(), session)
			}
			f.store.fail[tc.method] = errBoomSF
			if err := tc.call(f, session, code(t, secret, f.now)); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestStepUpWithAPasskeyFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"StepUpChallenge", "SetSessionMethods"} {
		t.Run(method, func(t *testing.T) {
			f := newFixture()
			id, session := f.withPasskey(t)
			ctx := context.Background()
			_, _ = f.StartStepUp(ctx, session)
			_, _ = f.StepUpPasskeyOptions(ctx, session)
			f.store.fail[method] = errBoomSF
			if err := f.StepUpWithPasskey(ctx, session, []byte("signed:login-"+id.String()+"|cred-1")); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestStepUpPasskeyOptionsFailWhenTheStoreDoes(t *testing.T) {
	f := newFixture()
	_, session := f.withPasskey(t)
	_, _ = f.StartStepUp(context.Background(), session)
	f.store.fail["StepUpChallenge"] = errBoomSF
	if _, err := f.StepUpPasskeyOptions(context.Background(), session); !errors.Is(err, errBoomSF) {
		t.Fatalf("err = %v", err)
	}
}

func TestTheUsersWithTwoFactorAreThoseWhoAddedIt(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "bob@example.org")
	id, _, _ := f.enrolled(t, "ada@example.org")
	if ids, err := f.UsersWithTwoFactor(context.Background()); err != nil || !slices.Equal(ids, []uuid.UUID{id}) {
		t.Fatalf("UsersWithTwoFactor = %v, %v", ids, err)
	}
}

func TestAMemberHeldBackByATwoFactorGroupIsMailedHowToGetTheirAppsBack(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if err := f.NotifyTwoFactorRequired(context.Background(), id, "Family"); err != nil {
		t.Fatal(err)
	}
	m := f.mailer.sent[len(f.mailer.sent)-1]
	if m.To != "ada@example.org" || m.Subject != "Family now requires two-factor" || !strings.Contains(m.Text, "https://auth.test/security/two-factor") {
		t.Fatalf("mail = %+v", m)
	}
}

func TestNoOneWhoIsGoneIsMailed(t *testing.T) {
	if err := newFixture().NotifyTwoFactorRequired(context.Background(), uuid.New(), "Family"); err == nil {
		t.Fatal("mailed someone who is gone")
	}
}
