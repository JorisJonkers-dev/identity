package app_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

// withPasskey signs Ada up and in and adds a passkey; it returns her id and the session.
func (f *fixture) withPasskey(t *testing.T) (uuid.UUID, string) {
	t.Helper()
	const email = "ada@example.org"
	ctx := context.Background()
	id := f.signedUp(t, email)
	session, err := f.SignIn(ctx, email, pw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.StartPasskey(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := f.ConfirmPasskey(ctx, id, session, "MacBook", []byte("made:reg-"+id.String()), pw); err != nil {
		t.Fatal(err)
	}
	return id, session
}

func TestAddingAPasskey(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id := f.signedUp(t, "ada@example.org")
	elsewhere, _ := f.SignIn(ctx, "ada@example.org", pw)
	session, _ := f.SignIn(ctx, "ada@example.org", pw)
	options, err := f.StartPasskey(ctx, id)
	if err != nil || string(options) != `{"publicKey":{}}` {
		t.Fatalf("StartPasskey = %s, %v", options, err)
	}
	if e := f.store.pending[id]; string(e.Ceremony) != "reg-"+id.String() || !e.ExpiresAt.Equal(t0.Add(10*time.Minute)) {
		t.Fatalf("enrolment = %+v", e)
	}
	if err := f.ConfirmPasskey(ctx, id, session, "MacBook", []byte("forged"), pw); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("a refused answer = %v", err)
	}
	if err := f.ConfirmPasskey(ctx, id, session, "MacBook", []byte("made:reg-"+id.String()), "a guess"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("without the password = %v", err)
	}
	if err := f.ConfirmPasskey(ctx, id, session, " MacBook ", []byte("made:reg-"+id.String()), pw); err != nil {
		t.Fatal(err)
	}
	if len(f.store.passkeys) != 1 || f.store.passkeys[0].Label.String() != "MacBook" || string(f.store.passkeys[0].CredentialID) != "cred-1" {
		t.Fatalf("passkeys = %+v", f.store.passkeys)
	}
	if s := f.store.sessions[sum(session)]; !slices.Equal(s.Methods, []string{"pwd", "hwk"}) || !app.ProvedSecondFactor(s.Methods) {
		t.Fatalf("methods = %v", s.Methods)
	}
	if _, err := f.Authenticate(ctx, elsewhere); !errors.Is(err, app.ErrNoSession) {
		t.Fatal("a password-only session outlived the new passkey")
	}
	if _, ok := f.store.pending[id]; ok {
		t.Fatal("the enrolment is still waiting")
	}
	if err := f.ConfirmPasskey(ctx, id, session, "MacBook", []byte("made:reg-"+id.String()), pw); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("confirming twice = %v", err)
	}

	// A User with a Second Factor adds another only from a session that proved one.
	f.store.sessions[sum("old")] = app.Session{TokenHash: []byte(sum("old")), UserID: id, ExpiresAt: f.now.Add(time.Hour), Methods: []string{"pwd"}}
	_, _ = f.StartPasskey(ctx, id)
	if err := f.ConfirmPasskey(ctx, id, "old", "iPhone", []byte("made:reg-"+id.String()), pw); !errors.Is(err, app.ErrSecondFactorNeeded) {
		t.Fatalf("adding from a password-only session = %v", err)
	}

	f.now = f.now.Add(10 * time.Minute)
	if err := f.ConfirmPasskey(ctx, id, session, "iPhone", []byte("made:reg-"+id.String()), pw); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("a late answer = %v", err)
	}
}

func TestSigningInWithAPasskey(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id, _ := f.withPasskey(t)
	f.now = t0.Add(time.Hour)

	challenge, err := f.SignIn(ctx, "ada@example.org", pw)
	if !errors.Is(err, app.ErrSecondFactorNeeded) {
		t.Fatalf("SignIn with a passkey = %v", err)
	}
	if kinds, err := f.ChallengeKinds(ctx, challenge); err != nil || !slices.Equal(kinds, []string{"passkey"}) {
		t.Fatalf("ChallengeKinds = %v, %v", kinds, err)
	}
	if _, err := f.CompleteSignInWithPasskey(ctx, challenge, []byte("signed:login-"+id.String()+"|cred-1")); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("an answer before the passkey step began = %v", err)
	}
	options, err := f.PasskeyOptions(ctx, challenge)
	if err != nil || len(options) == 0 {
		t.Fatalf("PasskeyOptions = %s, %v", options, err)
	}
	if _, err := f.CompleteSignInWithPasskey(ctx, challenge, []byte("signed:other|cred-1")); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("a refused assertion = %v", err)
	}
	_, _ = f.PasskeyOptions(ctx, challenge)
	session, err := f.CompleteSignInWithPasskey(ctx, challenge, []byte("signed:login-"+id.String()+"|cred-1"))
	if err != nil {
		t.Fatal(err)
	}
	if s := f.store.sessions[sum(session)]; !slices.Equal(s.Methods, []string{"pwd", "hwk"}) {
		t.Fatalf("methods = %v", s.Methods)
	}
	if p := f.store.passkeys[0]; string(p.Credential) != `{"id":"cred-1","count":1}` || p.LastUsedAt == nil {
		t.Fatalf("the passkey's counter was not kept: %+v", p)
	}
	if _, err := f.PasskeyOptions(ctx, challenge); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("options for a finished sign-in = %v", err)
	}
}

// One assertion opens one session: the ceremony is taken by the first request that uses it.
func TestAPasskeyAssertionWorksOnce(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id, _ := f.withPasskey(t)
	f.now = t0.Add(time.Hour)
	challenge, _ := f.SignIn(ctx, "ada@example.org", pw)
	_, _ = f.PasskeyOptions(ctx, challenge)
	assertion := []byte("signed:login-" + id.String() + "|cred-1")
	if _, err := f.CompleteSignInWithPasskey(ctx, challenge, []byte("signed:wrong|cred-1")); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("a refused assertion = %v", err)
	}
	// The refused answer spent the ceremony; the right one needs the next.
	if _, err := f.CompleteSignInWithPasskey(ctx, challenge, assertion); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("an assertion after the ceremony was taken = %v", err)
	}
	_, _ = f.PasskeyOptions(ctx, challenge)
	if _, err := f.CompleteSignInWithPasskey(ctx, challenge, []byte("signed:login-"+id.String()+"|ghost")); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("an assertion with a passkey the User does not have = %v", err)
	}

	// Two sign-ins with one stored counter: the one whose record went stale is refused.
	again, _ := f.SignIn(ctx, "ada@example.org", pw)
	_, _ = f.PasskeyOptions(ctx, again)
	f.wa.staleUse = func() { f.store.passkeys[0].Credential = []byte(`{"id":"cred-1","count":8}`) }
	if _, err := f.CompleteSignInWithPasskey(ctx, again, assertion); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("a sign-in that lost the counter = %v", err)
	}
	if _, err := f.CompleteSignInWithPasskey(ctx, challenge, nil); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("a finished sign-in = %v", err)
	}
}

func TestAUserWithBothFactorsIsOfferedBoth(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_, secret, session := f.enrolled(t, "ada@example.org")
	id, _ := f.UserByEmail(ctx, "ada@example.org")
	_, _ = f.StartPasskey(ctx, id.ID)
	f.now = f.now.Add(time.Minute)
	if err := f.ConfirmPasskey(ctx, id.ID, session, "MacBook", []byte("made:reg-"+id.ID.String()), pw); err != nil {
		t.Fatal(err)
	}
	factors, _ := f.SecondFactors(ctx, id.ID)
	if len(factors) != 2 || factors[0].Kind != app.KindTOTP || factors[1].Kind != app.KindPasskey {
		t.Fatalf("factors = %+v", factors)
	}
	challenge, _ := f.SignIn(ctx, "ada@example.org", pw)
	if kinds, _ := f.ChallengeKinds(ctx, challenge); !slices.Equal(kinds, []string{"totp", "passkey"}) {
		t.Fatalf("kinds = %v", kinds)
	}
	f.now = f.now.Add(time.Minute)
	if _, err := f.CompleteSignIn(ctx, challenge, code(t, secret, f.now)); err != nil {
		t.Fatalf("a code still signs in = %v", err)
	}

	// Removing goes for whichever kind the id names.
	passkeyID := factors[1].ID
	if err := f.RemoveFactor(ctx, id.ID, passkeyID, session, pw); err != nil || len(f.store.passkeys) != 0 {
		t.Fatalf("removing the passkey = %v, %d left", err, len(f.store.passkeys))
	}
}

func TestTwoAuthenticatorAppsAreOneKind(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id, _, session := f.enrolled(t, "ada@example.org")
	setup, _ := f.StartTOTP(ctx, id)
	f.now = f.now.Add(time.Minute)
	if err := f.ConfirmTOTP(ctx, id, session, "Tablet", code(t, setup.Secret, f.now), pw); err != nil {
		t.Fatal(err)
	}
	if kinds, err := f.SecondFactorKinds(ctx, id); err != nil || !slices.Equal(kinds, []string{"totp"}) {
		t.Fatalf("kinds = %v, %v", kinds, err)
	}
}

func TestPasskeyOptionsNeedAPasskeyAndALiveChallenge(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.enrolled(t, "ada@example.org")
	challenge, _ := f.SignIn(ctx, "ada@example.org", pw)
	if _, err := f.PasskeyOptions(ctx, challenge); !errors.Is(err, app.ErrNoPasskeys) {
		t.Fatalf("options for a User without a passkey = %v", err)
	}
	if _, err := f.PasskeyOptions(ctx, "no such challenge"); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("options for no challenge = %v", err)
	}
	if _, err := f.ChallengeKinds(ctx, "no such challenge"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("kinds for no challenge = %v", err)
	}
}

var errBoomPK = errors.New("boom")

func TestWithoutWebAuthnPasskeysAreUnavailable(t *testing.T) {
	ctx := context.Background()
	bare := app.New(newStore(), &hasher{}, &mailer{}, time.Now, bytes.NewReader(nil), "")
	for name, call := range map[string]func() error{
		"starting one":           func() error { _, err := bare.StartPasskey(ctx, uuid.New()); return err },
		"confirming one":         func() error { return bare.ConfirmPasskey(ctx, uuid.New(), "s", "MacBook", nil, pw) },
		"asking for its options": func() error { _, err := bare.PasskeyOptions(ctx, "c"); return err },
		"signing in with one":    func() error { _, err := bare.CompleteSignInWithPasskey(ctx, "c", nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, app.ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestAPasskeyNeedsAName(t *testing.T) {
	bare := app.New(newStore(), &hasher{}, &mailer{}, time.Now, bytes.NewReader(nil), "")
	if err := bare.ConfirmPasskey(context.Background(), uuid.New(), "s", " ", nil, pw); err == nil {
		t.Fatal("an empty passkey name was accepted")
	}
}

func TestStartingAPasskeyFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"UserByID", "Passkeys", "SavePasskeyEnrolment"} {
		t.Run(method, func(t *testing.T) {
			f := newFixture()
			id := f.signedUp(t, "ada@example.org")
			f.store.fail[method] = errBoomPK
			if _, err := f.StartPasskey(context.Background(), id); !errors.Is(err, errBoomPK) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestStartingAPasskeyFailsWhenWebAuthnDoes(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	f.wa.failBegin = errBoomPK
	if _, err := f.StartPasskey(context.Background(), id); !errors.Is(err, errBoomPK) {
		t.Fatalf("err = %v", err)
	}
}

// addingPasskey is a fixture where Ada, signed in, has started adding a passkey.
func addingPasskey(t *testing.T) (*fixture, uuid.UUID, string) {
	t.Helper()
	ctx := context.Background()
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(ctx, "ada@example.org", pw)
	_, _ = f.StartPasskey(ctx, id)
	return f, id, session
}

func TestConfirmingAPasskeyFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"PasskeyEnrolment", "CreatePasskey", "DeletePasskeyEnrolment", "SetSessionMethods", "Passkeys"} {
		t.Run(method, func(t *testing.T) {
			f, id, session := addingPasskey(t)
			f.store.fail[method] = errBoomPK
			if err := f.ConfirmPasskey(context.Background(), id, session, "MacBook", []byte("made:reg-"+id.String()), pw); !errors.Is(err, errBoomPK) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestConfirmingAPasskeyFailsWhenThePasskeysCannotBeReadForTheCeremony(t *testing.T) {
	f, id, session := addingPasskey(t)
	one := 1
	f.store.passkeyReads = &one
	if err := f.ConfirmPasskey(context.Background(), id, session, "MacBook", []byte("made:reg-"+id.String()), pw); err == nil {
		t.Fatal("a passkey was kept")
	}
}

func TestNoPasskeyIsKeptWithoutRandomnessForItsID(t *testing.T) {
	f, id, session := addingPasskey(t)
	f.dry = true
	if err := f.ConfirmPasskey(context.Background(), id, session, "MacBook", []byte("made:reg-"+id.String()), pw); err == nil {
		t.Fatal("a passkey was kept without randomness")
	}
}

// passkeyStep is a fixture an hour after Ada added a passkey, mid sign-in.
func passkeyStep(t *testing.T) (*fixture, uuid.UUID, string) {
	t.Helper()
	f := newFixture()
	id, _ := f.withPasskey(t)
	f.now = t0.Add(time.Hour)
	challenge, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	return f, id, challenge
}

func TestThePasskeyOptionsFailWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"ChallengeCeremony", "UserByID", "SetChallengeCeremony"} {
		t.Run(method, func(t *testing.T) {
			f, _, challenge := passkeyStep(t)
			f.store.fail[method] = errBoomPK
			if _, err := f.PasskeyOptions(context.Background(), challenge); !errors.Is(err, errBoomPK) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestThePasskeyOptionsOfAChallengeThatExpiredMeanwhileAreRefused(t *testing.T) {
	f, _, challenge := passkeyStep(t)
	f.store.fail["SetChallengeCeremony"] = app.ErrNotFound
	if _, err := f.PasskeyOptions(context.Background(), challenge); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("err = %v, want ErrInvalidToken", err)
	}
}

func TestThePasskeyOptionsFailWhenWebAuthnDoes(t *testing.T) {
	f, _, challenge := passkeyStep(t)
	f.wa.failBegin = errBoomPK
	if _, err := f.PasskeyOptions(context.Background(), challenge); !errors.Is(err, errBoomPK) {
		t.Fatalf("err = %v", err)
	}
}

func TestFinishingASignInWithAPasskeyFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"SpendChallengeAttempt", "TakeChallengeCeremony", "Passkeys", "UsePasskey", "DeleteChallenge", "CreateSession"} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			f, id, challenge := passkeyStep(t)
			_, _ = f.PasskeyOptions(ctx, challenge)
			f.store.fail[method] = errBoomPK
			if _, err := f.CompleteSignInWithPasskey(ctx, challenge, []byte("signed:login-"+id.String()+"|cred-1")); !errors.Is(err, errBoomPK) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestTheSecondFactorKindsFailWhenThePasskeysCannotBeRead(t *testing.T) {
	f, _, challenge := passkeyStep(t)
	f.store.fail["Passkeys"] = errBoomPK
	if _, err := f.ChallengeKinds(context.Background(), challenge); !errors.Is(err, errBoomPK) {
		t.Fatalf("err = %v", err)
	}
}

func TestSigningInFailsWhenTheAuthenticatorsCannotBeRead(t *testing.T) {
	f, _, _ := passkeyStep(t)
	f.store.fail["TOTPFactors"] = errBoomPK
	if _, err := f.SignIn(context.Background(), "ada@example.org", pw); !errors.Is(err, errBoomPK) {
		t.Fatalf("err = %v", err)
	}
}

func TestRemovingAPasskeyFailsWhenTheStoreDoes(t *testing.T) {
	f := newFixture()
	id, session := f.withPasskey(t)
	f.store.fail["DeletePasskey"] = errBoomPK
	if err := f.RemoveFactor(context.Background(), id, f.store.passkeys[0].ID, session, pw); !errors.Is(err, errBoomPK) {
		t.Fatalf("err = %v", err)
	}
}
