package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

func code(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	c, err := totp.GenerateCode(secret, at)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sum(token string) string {
	h := sha256.Sum256([]byte(token))
	return string(h[:])
}

// enrolled signs a User up and in, and enrols an authenticator; it returns the User, the secret
// and the session it was enrolled in.
func (f *fixture) enrolled(t *testing.T, email string) (uuid.UUID, string, string) {
	t.Helper()
	ctx := context.Background()
	id := f.signedUp(t, email)
	session, err := f.SignIn(ctx, email, pw)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := f.StartTOTP(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.ConfirmTOTP(ctx, id, session, "Phone", code(t, setup.Secret, f.now), pw); err != nil {
		t.Fatal(err)
	}
	return id, setup.Secret, session
}

func TestEnrollingAnAuthenticatorKeepsItsSecretSealed(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id := f.signedUp(t, "ada@example.org")
	session, _ := f.SignIn(ctx, "ada@example.org", pw)
	if s := f.store.sessions[sum(session)]; !slices.Equal(s.Methods, []string{"pwd"}) {
		t.Fatalf("a password sign-in's methods = %v", s.Methods)
	}

	setup, err := f.StartTOTP(ctx, id)
	if err != nil || len(setup.Secret) != 32 || !strings.Contains(setup.URI, "jorisjonkers.dev:ada@example.org") {
		t.Fatalf("StartTOTP = %+v, %v", setup, err)
	}
	e := f.store.enrolments[id]
	if !strings.HasPrefix(e.SealedSecret, "vault:v1:") || strings.Contains(e.SealedSecret, setup.Secret) || !e.ExpiresAt.Equal(t0.Add(10*time.Minute)) {
		t.Fatalf("enrolment = %+v", e)
	}

	if err := f.ConfirmTOTP(ctx, id, session, "Phone", "000000", pw); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("a wrong first code = %v", err)
	}
	if err := f.ConfirmTOTP(ctx, id, session, " ", code(t, setup.Secret, f.now), pw); !errors.Is(err, domain.ErrInvalidLabel) {
		t.Fatalf("an empty name = %v", err)
	}
	if err := f.ConfirmTOTP(ctx, id, session, " Phone ", code(t, setup.Secret, f.now), pw); err != nil {
		t.Fatal(err)
	}
	factors, _ := f.SecondFactors(ctx, id)
	if len(factors) != 1 || factors[0].Label.String() != "Phone" || factors[0].Kind != app.KindTOTP ||
		f.store.factors[0].SealedSecret != e.SealedSecret || f.store.factors[0].LastStep != t0.Unix()/30 {
		t.Fatalf("factors = %+v, stored %+v", factors, f.store.factors)
	}
	if _, ok := f.store.enrolments[id]; ok {
		t.Fatal("the enrolment is still waiting")
	}
	if s := f.store.sessions[sum(session)]; !app.ProvedSecondFactor(s.Methods) {
		t.Fatalf("the session it was confirmed in has methods %v", s.Methods)
	}
	if err := f.ConfirmTOTP(ctx, id, session, "Phone", code(t, setup.Secret, f.now), pw); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("confirming twice = %v", err)
	}
}

func TestAnEnrolmentExpires(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id := f.signedUp(t, "ada@example.org")
	setup, _ := f.StartTOTP(ctx, id)
	f.now = t0.Add(10 * time.Minute)
	if err := f.ConfirmTOTP(ctx, id, "s", "Phone", code(t, setup.Secret, f.now), pw); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("a late confirmation = %v", err)
	}
}

func TestSignInAsksForTheCodeAndTakesItOnce(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_, secret, _ := f.enrolled(t, "ada@example.org")
	f.now = t0.Add(time.Hour)

	challenge, err := f.SignIn(ctx, "ada@example.org", pw)
	if !errors.Is(err, app.ErrSecondFactorNeeded) || challenge == "" {
		t.Fatalf("SignIn with an authenticator = %q, %v", challenge, err)
	}
	c, ok := f.store.challenges[sum(challenge)]
	if !ok || !c.ExpiresAt.Equal(f.now.Add(5*time.Minute)) {
		t.Fatalf("challenge = %+v, %v", c, ok)
	}
	if _, ok := f.store.sessions[sum(challenge)]; ok {
		t.Fatal("the challenge is a session")
	}

	if _, err := f.CompleteSignIn(ctx, challenge, "000000"); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("a wrong code = %v", err)
	}
	good := code(t, secret, f.now)
	session, err := f.CompleteSignIn(ctx, challenge, good)
	if err != nil {
		t.Fatal(err)
	}
	if s := f.store.sessions[sum(session)]; !slices.Equal(s.Methods, []string{"pwd", "otp"}) {
		t.Fatalf("methods = %v", s.Methods)
	}
	if _, err := f.CompleteSignIn(ctx, challenge, good); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("a used challenge = %v", err)
	}

	again, _ := f.SignIn(ctx, "ada@example.org", pw)
	if _, err := f.CompleteSignIn(ctx, again, good); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("a replayed code = %v", err)
	}
}

func TestAChallengeExpiresAfterFiveMinutes(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_, secret, _ := f.enrolled(t, "ada@example.org")
	late, _ := f.SignIn(ctx, "ada@example.org", pw)
	f.now = f.now.Add(5 * time.Minute)
	if _, err := f.CompleteSignIn(ctx, late, code(t, secret, f.now)); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("a late code = %v", err)
	}
}

// Knowing the password must not buy unlimited guesses at the code: wrong codes count against the
// User across sign-ins, five per fifteen minutes.
func TestWrongCodesCountAcrossSignIns(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_, secret, _ := f.enrolled(t, "ada@example.org")
	f.now = t0.Add(time.Hour)
	for i := range 5 {
		challenge, err := f.SignIn(ctx, "ada@example.org", pw)
		if !errors.Is(err, app.ErrSecondFactorNeeded) {
			t.Fatalf("sign-in %d = %v", i, err)
		}
		_, _ = f.CompleteSignIn(ctx, challenge, "000000")
		f.now = f.now.Add(time.Minute)
	}
	sixth, _ := f.SignIn(ctx, "ada@example.org", pw)
	if _, err := f.CompleteSignIn(ctx, sixth, code(t, secret, f.now)); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("a right code after five wrong ones in a new sign-in = %v", err)
	}
	if len(f.store.challenges) != 1 {
		t.Fatalf("%d challenges for one User", len(f.store.challenges))
	}
	f.now = t0.Add(time.Hour + 15*time.Minute)
	later, _ := f.SignIn(ctx, "ada@example.org", pw)
	if _, err := f.CompleteSignIn(ctx, later, code(t, secret, f.now)); err != nil {
		t.Fatalf("a right code once the window has passed = %v", err)
	}
}

// A session someone took, without the password, must not make itself a Second Factor session by
// adding its own authenticator.
func TestATakenSessionCannotProveItselfByEnrolling(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id := f.signedUp(t, "ada@example.org")
	taken, _ := f.SignIn(ctx, "ada@example.org", pw)
	setup, _ := f.StartTOTP(ctx, id)
	if err := f.ConfirmTOTP(ctx, id, taken, "Mine", code(t, setup.Secret, f.now), "a guess at it"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("enrolling without the password = %v", err)
	}
	if s := f.store.sessions[sum(taken)]; app.ProvedSecondFactor(s.Methods) || len(f.store.factors) != 0 {
		t.Fatalf("a refused enrolment left methods %v and %d factors", s.Methods, len(f.store.factors))
	}

	// A User with an authenticator, in a session from before they had it, adds another only after
	// signing in with the first.
	_, _, _ = f.enrolled(t, "bob@example.org")
	bob, _ := f.UserByEmail(ctx, "bob@example.org")
	f.store.sessions[sum("old")] = app.Session{TokenHash: []byte(sum("old")), UserID: bob.ID, ExpiresAt: f.now.Add(time.Hour), Methods: []string{"pwd"}}
	more, _ := f.StartTOTP(ctx, bob.ID)
	f.now = f.now.Add(time.Minute)
	if err := f.ConfirmTOTP(ctx, bob.ID, "old", "Tablet", code(t, more.Secret, f.now), pw); !errors.Is(err, app.ErrSecondFactorNeeded) {
		t.Fatalf("adding a second authenticator from a password-only session = %v", err)
	}
}

func TestTwoSignInsRacingForOneCodeLetOneThrough(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_, secret, _ := f.enrolled(t, "ada@example.org")
	f.now = t0.Add(time.Hour)
	challenge, _ := f.SignIn(ctx, "ada@example.org", pw)
	f.store.stale = true
	if _, err := f.CompleteSignIn(ctx, challenge, code(t, secret, f.now)); !errors.Is(err, app.ErrInvalidCode) {
		t.Fatalf("the losing sign-in = %v", err)
	}
}

func TestRemovingAnAuthenticatorNeedsThePasswordAndAProvedSession(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id, _, session := f.enrolled(t, "ada@example.org")
	factorID := f.store.factors[0].ID
	if err := f.RemoveFactor(ctx, id, factorID, session, "wrong password!"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("a wrong password = %v", err)
	}
	if err := f.RemoveFactor(ctx, uuid.New(), factorID, session, pw); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("someone else = %v", err)
	}

	// A session from before the User enrolled, with the password, must not take the factor away.
	f.store.sessions[sum("old")] = app.Session{TokenHash: []byte(sum("old")), UserID: id, ExpiresAt: f.now.Add(time.Hour), Methods: []string{"pwd"}}
	if err := f.RemoveFactor(ctx, id, factorID, "old", pw); !errors.Is(err, app.ErrSecondFactorNeeded) || len(f.store.factors) != 1 {
		t.Fatalf("removing from a password-only session = %v, %d left", err, len(f.store.factors))
	}

	if err := f.RemoveFactor(ctx, id, factorID, session, pw); err != nil || len(f.store.factors) != 0 {
		t.Fatalf("RemoveTOTP = %v, %d left", err, len(f.store.factors))
	}
	if err := f.RemoveFactor(ctx, id, factorID, session, pw); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("removing it twice = %v", err)
	}
	if _, err := f.SignIn(ctx, "ada@example.org", pw); err != nil {
		t.Fatalf("a sign-in without an authenticator = %v", err)
	}
}

func TestEnrollingEndsEveryOtherSession(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id := f.signedUp(t, "ada@example.org")
	elsewhere, _ := f.SignIn(ctx, "ada@example.org", pw)
	here, _ := f.SignIn(ctx, "ada@example.org", pw)
	setup, _ := f.StartTOTP(ctx, id)
	if err := f.ConfirmTOTP(ctx, id, here, "Phone", code(t, setup.Secret, f.now), pw); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Authenticate(ctx, elsewhere); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("a password-only session outlived the new authenticator: %v", err)
	}
	if _, err := f.Authenticate(ctx, here); err != nil {
		t.Fatalf("the session it was added in ended: %v", err)
	}
}

func TestAnAdminSessionLastsTwelveHours(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	id := f.signedUp(t, "ada@example.org")
	f.admins.ids[id] = true
	token, _ := f.SignIn(ctx, "ada@example.org", pw)
	if s := f.store.sessions[sum(token)]; !s.ExpiresAt.Equal(t0.Add(12 * time.Hour)) {
		t.Fatalf("an Admin's session expires %v", s.ExpiresAt)
	}
	f.now = t0.Add(11 * time.Hour)
	if _, err := f.Authenticate(ctx, token); err != nil {
		t.Fatal(err)
	}
	if s := f.store.sessions[sum(token)]; !s.ExpiresAt.Equal(t0.Add(12 * time.Hour)) {
		t.Fatalf("use extended an Admin's session to %v", s.ExpiresAt)
	}
	f.now = t0.Add(12 * time.Hour)
	if _, err := f.Authenticate(ctx, token); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("after 12 hours = %v", err)
	}

	// Promoted with a 30-day session open, the session ends 12 hours after it began.
	bob := f.signedUp(t, "bob@example.org")
	f.now = t0
	older, _ := f.SignIn(ctx, "bob@example.org", pw)
	f.admins.ids[bob] = true
	f.now = t0.Add(13 * time.Hour)
	if _, err := f.Authenticate(ctx, older); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("a session older than 12 hours of a new Admin = %v", err)
	}
}

var errBoomSF = errors.New("boom")

func TestWithoutVaultAuthenticatorAppsAreUnavailable(t *testing.T) {
	ctx := context.Background()
	bare := app.New(newStore(), &hasher{}, &mailer{}, time.Now, bytes.NewReader(nil), "")
	for name, call := range map[string]func() error{
		"starting one":         func() error { _, err := bare.StartTOTP(ctx, uuid.New()); return err },
		"confirming one":       func() error { return bare.ConfirmTOTP(ctx, uuid.New(), "s", "Phone", "123456", pw) },
		"signing in with code": func() error { _, err := bare.CompleteSignIn(ctx, "c", "123456"); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, app.ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestSigningInWithASecondFactorFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"TOTPFactors", "SaveChallenge"} {
		t.Run(method, func(t *testing.T) {
			f := newFixture()
			f.enrolled(t, "ada@example.org")
			f.store.fail[method] = errBoomSF
			if _, err := f.SignIn(context.Background(), "ada@example.org", pw); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// codeStep is a fixture an hour after Ada enrolled, mid sign-in, with the code to finish it.
func codeStep(t *testing.T) (*fixture, string, string) {
	t.Helper()
	f := newFixture()
	_, secret, _ := f.enrolled(t, "ada@example.org")
	f.now = t0.Add(time.Hour)
	challenge, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	return f, challenge, code(t, secret, f.now)
}

func TestFinishingASignInWithACodeFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"SpendChallengeAttempt", "TOTPFactors", "UseTOTPStep", "DeleteChallenge", "CreateSession"} {
		t.Run(method, func(t *testing.T) {
			f, challenge, c := codeStep(t)
			f.store.fail[method] = errBoomSF
			if _, err := f.CompleteSignIn(context.Background(), challenge, c); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestFinishingASignInWithACodeFailsWhenVaultCannotOpenTheSecret(t *testing.T) {
	f, challenge, c := codeStep(t)
	f.sealer.failOpen = errBoomSF
	if _, err := f.CompleteSignIn(context.Background(), challenge, c); !errors.Is(err, errBoomSF) {
		t.Fatalf("err = %v", err)
	}
}

func TestStartingAnAuthenticatorFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"UserByID", "SaveTOTPEnrolment"} {
		t.Run(method, func(t *testing.T) {
			f := newFixture()
			id := f.signedUp(t, "ada@example.org")
			f.store.fail[method] = errBoomSF
			if _, err := f.StartTOTP(context.Background(), id); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestStartingAnAuthenticatorFailsWhenVaultCannotSealTheSecret(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	f.sealer.failSeal = errBoomSF
	if _, err := f.StartTOTP(context.Background(), id); !errors.Is(err, errBoomSF) {
		t.Fatalf("err = %v", err)
	}
}

func TestStartingAnAuthenticatorForNobodyFails(t *testing.T) {
	broken := app.New(newStore(), &hasher{}, &mailer{}, time.Now, bytes.NewReader(nil), "", app.WithSealer(&sealer{}))
	if _, err := broken.StartTOTP(context.Background(), uuid.New()); err == nil {
		t.Fatal("an authenticator was started for nobody")
	}
}

func TestConfirmingAnAuthenticatorFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"TOTPEnrolment", "CreateTOTPFactor", "DeleteTOTPEnrolment", "SetSessionMethods", "TOTPFactors", "DeleteOtherSessions", "UserByID"} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			f := newFixture()
			id := f.signedUp(t, "ada@example.org")
			setup, _ := f.StartTOTP(ctx, id)
			f.store.fail[method] = errBoomSF
			if err := f.ConfirmTOTP(ctx, id, "s", "Phone", code(t, setup.Secret, f.now), pw); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestConfirmingASecondAuthenticatorFailsWhenTheSessionCannotBeRead(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	id, _, session := f.enrolled(t, "ada@example.org")
	another, _ := f.StartTOTP(ctx, id)
	f.store.fail["SessionByHash"] = errBoomSF
	if err := f.ConfirmTOTP(ctx, id, session, "Tablet", code(t, another.Secret, f.now.Add(time.Minute)), pw); !errors.Is(err, errBoomSF) {
		t.Fatalf("err = %v", err)
	}
}

func TestConfirmingAnAuthenticatorFailsWhenVaultCannotOpenTheSecret(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	setup, _ := f.StartTOTP(ctx, id)
	f.sealer.failOpen = errBoomSF
	if err := f.ConfirmTOTP(ctx, id, "s", "Phone", code(t, setup.Secret, f.now), pw); !errors.Is(err, errBoomSF) {
		t.Fatalf("err = %v", err)
	}
}

func TestRemovingAnAuthenticatorFailsWhenTheStoreDoes(t *testing.T) {
	for _, method := range []string{"UserByID", "DeleteTOTPFactor", "SessionByHash"} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			f := newFixture()
			id, _, session := f.enrolled(t, "ada@example.org")
			f.store.fail[method] = errBoomSF
			if err := f.RemoveFactor(ctx, id, f.store.factors[0].ID, session, pw); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestRemovingAnAuthenticatorFailsWhenThePasswordCannotBeChecked(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	id, _, session := f.enrolled(t, "ada@example.org")
	f.hasher.fail = errBoomSF
	if err := f.RemoveFactor(ctx, id, f.store.factors[0].ID, session, pw); !errors.Is(err, errBoomSF) {
		t.Fatalf("err = %v", err)
	}
}

func TestSigningInFailsWhenTheAdminCheckCannotBeMade(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	f.admins.err = errBoomSF
	if _, err := f.SignIn(context.Background(), "ada@example.org", pw); !errors.Is(err, errBoomSF) {
		t.Fatalf("err = %v", err)
	}
}

func TestASessionCannotBeTrustedWhenTheAdminCheckCannotBeMade(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	token, _ := f.SignIn(ctx, "ada@example.org", pw)
	f.admins.err = errBoomSF
	if _, err := f.Authenticate(ctx, token); !errors.Is(err, errBoomSF) {
		t.Fatalf("err = %v", err)
	}
}

func TestWithoutRandomnessNothingSecondFactorIsMade(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	id, _, session := f.enrolled(t, "ada@example.org")
	f.dry = true
	if _, err := f.SignIn(ctx, "ada@example.org", pw); err == nil || errors.Is(err, app.ErrSecondFactorNeeded) {
		t.Errorf("a challenge without randomness = %v", err)
	}
	if _, err := f.StartTOTP(ctx, id); err == nil {
		t.Error("a secret without randomness")
	}
	f.dry = false
	setup, _ := f.StartTOTP(ctx, id)
	f.dry = true
	f.now = f.now.Add(time.Minute)
	if err := f.ConfirmTOTP(ctx, id, session, "Laptop", code(t, setup.Secret, f.now), pw); err == nil {
		t.Error("a factor id without randomness")
	}
}

func TestWithoutAdminsEverySessionLastsThirtyDays(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	plain := app.New(f.store, f.hasher, f.mailer, func() time.Time { return t0 }, bytes.NewReader(bytes.Repeat([]byte{9}, 64)), "")
	token, err := plain.SignIn(ctx, "ada@example.org", pw)
	if err != nil {
		t.Fatal(err)
	}
	if s := f.store.sessions[sum(token)]; !s.ExpiresAt.Equal(t0.Add(30 * 24 * time.Hour)) {
		t.Fatalf("expires %v", s.ExpiresAt)
	}
}
