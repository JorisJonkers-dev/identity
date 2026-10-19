//go:build integration

package persistence_test

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/users/adapters/persistence"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

func open(t *testing.T) (*persistence.Store, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return persistence.New(queries.New(pool)), pool
}

func user(t *testing.T) domain.User {
	t.Helper()
	e, err := domain.NewEmail("ada@example.org")
	if err != nil {
		t.Fatal(err)
	}
	n, _ := domain.NewName("Ada")
	return domain.User{ID: uuid.New(), Email: e, Name: n, PasswordHash: "stored-hash-1", CreatedAt: t0}
}

// stored is a store with Ada, as a User, in it.
func stored(t *testing.T) (*persistence.Store, *pgxpool.Pool, domain.User) {
	t.Helper()
	s, pool := open(t)
	u := user(t)
	if err := s.CreateUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return s, pool, u
}

func session(u domain.User, hash string) app.Session {
	return app.Session{TokenHash: []byte(hash), ID: uuid.New(), UserID: u.ID, CreatedAt: t0, LastSeenAt: t0, ExpiresAt: t0.Add(time.Hour), Methods: []string{"pwd"}}
}

func TestAUserIsFoundByEmail(t *testing.T) {
	s, _, u := stored(t)
	got, err := s.UserByEmail(context.Background(), u.Email)
	if err != nil || got.ID != u.ID || !got.CreatedAt.Equal(t0) {
		t.Fatalf("UserByEmail = %+v, %v", got, err)
	}
}

func TestTwoUsersCannotShareAnEmail(t *testing.T) {
	s, _, _ := stored(t)
	if err := s.CreateUser(context.Background(), user(t)); !errors.Is(err, app.ErrEmailTaken) {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
}

func TestAnUnknownEmailIsNotFound(t *testing.T) {
	s, _ := open(t)
	missing, _ := domain.NewEmail("nobody@example.org")
	if _, err := s.UserByEmail(context.Background(), missing); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestAnUnknownUserIDIsNotFound(t *testing.T) {
	s, _ := open(t)
	if _, err := s.UserByID(context.Background(), uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestEveryUserIsListed(t *testing.T) {
	s, _, u := stored(t)
	if all, err := s.Users(context.Background()); err != nil || len(all) != 1 || all[0].ID != u.ID {
		t.Fatalf("Users = %+v, %v", all, err)
	}
}

func TestAUsersNameAndPasswordHashChange(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	newName, _ := domain.NewName("Ada King")
	if err := s.UpdateName(ctx, u.ID, newName); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePasswordHash(ctx, u.ID, "stored-hash-2"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.UserByID(ctx, u.ID); err != nil || got.Name != newName || got.PasswordHash != "stored-hash-2" {
		t.Fatalf("UserByID = %+v, %v", got, err)
	}
}

func TestARowTheDomainRefusesIsAnError(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	// The database checks case and length, not the @; the domain refuses this when reading it.
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name, password_hash, created_at) VALUES ($1, 'not-an-email', 'Ada', 'h', now())`, uuid.New()); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT id FROM users WHERE email = 'not-an-email'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserByID(ctx, id); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("UserByID = %v, want ErrInvalidEmail", err)
	}
	if _, err := s.Users(ctx); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("Users = %v, want ErrInvalidEmail", err)
	}
}

func TestASignupIsKeptByItsTokenHash(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	u := user(t)
	su := app.Signup{TokenHash: []byte("hash-of-a-token"), Email: u.Email, Name: u.Name, PasswordHash: "stored-hash-1", ExpiresAt: t0.Add(24 * time.Hour)}
	if err := s.CreateSignup(ctx, su); err != nil {
		t.Fatal(err)
	}
	got, err := s.SignupByHash(ctx, su.TokenHash)
	if err != nil || got.Email != su.Email || got.Name != su.Name || got.PasswordHash != su.PasswordHash || !got.ExpiresAt.Equal(su.ExpiresAt) {
		t.Fatalf("SignupByHash = %+v, %v", got, err)
	}
}

func TestADeletedSignupIsNotFound(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	u := user(t)
	su := app.Signup{TokenHash: []byte("hash-of-a-token"), Email: u.Email, Name: u.Name, PasswordHash: "h", ExpiresAt: t0}
	_ = s.CreateSignup(ctx, su)
	if err := s.DeleteSignup(ctx, su.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignupByHash(ctx, su.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestASignupRowTheDomainRefusesIsAnError(t *testing.T) {
	for name, row := range map[string][2]string{"a bad email": {"not-an-email", "Ada"}, "no name": {"ada@example.org", ""}} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			s, pool := open(t)
			hash := []byte("bad")
			if _, err := pool.Exec(ctx, `INSERT INTO signups (token_hash, email, name, password_hash, expires_at) VALUES ($1, $2, $3, 'h', now())`, hash, row[0], row[1]); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SignupByHash(ctx, hash); err == nil {
				t.Fatal("the row was read")
			}
		})
	}
}

func TestATouchedSessionReadsBackWithItsNewTimes(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	sess := session(u, "hash")
	_ = s.CreateSession(ctx, sess)
	if err := s.TouchSession(ctx, sess.TokenHash, t0.Add(time.Minute), t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionByHash(ctx, sess.TokenHash)
	if err != nil || !bytes.Equal(got.TokenHash, sess.TokenHash) || got.ID != sess.ID || got.UserID != u.ID ||
		!got.CreatedAt.Equal(t0) || !got.LastSeenAt.Equal(t0.Add(time.Minute)) || !got.ExpiresAt.Equal(t0.Add(2*time.Hour)) ||
		!slices.Equal(got.Methods, []string{"pwd"}) {
		t.Fatalf("SessionByHash = %+v, %v", got, err)
	}
}

func TestASessionRecordsTheSecondFactorItProved(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	sess := session(u, "hash")
	_ = s.CreateSession(ctx, sess)
	at := t0.Add(time.Minute)
	if err := s.SetSessionMethods(ctx, sess.TokenHash, []string{"pwd", "otp"}, at); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.SessionByHash(ctx, sess.TokenHash); !slices.Equal(got.Methods, []string{"pwd", "otp"}) || got.TwoFactorAt == nil || !got.TwoFactorAt.Equal(at) {
		t.Fatalf("methods = %v, at %v", got.Methods, got.TwoFactorAt)
	}
}

func TestASessionStartedWithASecondFactorSaysWhen(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	sess := session(u, "hash")
	sess.Methods, sess.TwoFactorAt = []string{"pwd", "otp"}, &t0
	_ = s.CreateSession(ctx, sess)
	if got, _ := s.SessionByID(ctx, sess.ID); got.TwoFactorAt == nil || !got.TwoFactorAt.Equal(t0) {
		t.Fatalf("two-factor at %v", got.TwoFactorAt)
	}
}

func TestAStepUpChallengeIsFoundByItsSession(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	sess := session(u, "hash")
	_ = s.CreateSession(ctx, sess)
	c := app.Challenge{TokenHash: []byte("step-up"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0, SessionHash: sess.TokenHash}
	_ = s.SaveChallenge(ctx, c, t0.Add(-15*time.Minute))
	if got, err := s.StepUpChallenge(ctx, sess.TokenHash); err != nil || string(got) != "step-up" {
		t.Fatalf("StepUpChallenge = %q, %v", got, err)
	}
}

func TestASignInChallengeIsNoSessionsStepUp(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	sess := session(u, "hash")
	_ = s.CreateSession(ctx, sess)
	_ = s.SaveChallenge(ctx, app.Challenge{TokenHash: []byte("sign-in"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0}, t0.Add(-15*time.Minute))
	if _, err := s.StepUpChallenge(ctx, sess.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("StepUpChallenge = %v", err)
	}
}

func TestASessionIsFoundByItsID(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	sess := session(u, "hash")
	_ = s.CreateSession(ctx, sess)
	if got, err := s.SessionByID(ctx, sess.ID); err != nil || !bytes.Equal(got.TokenHash, sess.TokenHash) {
		t.Fatalf("SessionByID = %+v, %v", got, err)
	}
}

func TestAnUnknownSessionIDIsNotFound(t *testing.T) {
	s, _ := open(t)
	if _, err := s.SessionByID(context.Background(), uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestEndingOtherSessionsKeepsTheOneNamedAndSaysWhichEnded(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	keep, other, third := session(u, "keep"), session(u, "other"), session(u, "third")
	for _, sess := range []app.Session{keep, other, third} {
		_ = s.CreateSession(ctx, sess)
	}
	ended, err := s.DeleteOtherSessions(ctx, u.ID, keep.TokenHash)
	if err != nil || len(ended) != 2 || !slices.Contains(ended, other.ID) || !slices.Contains(ended, third.ID) {
		t.Fatalf("ended %v, %v; want the other two", ended, err)
	}
	if _, err := s.SessionByHash(ctx, keep.TokenHash); err != nil {
		t.Fatalf("the kept session ended: %v", err)
	}
	if _, err := s.SessionByHash(ctx, other.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("another session survived: %v", err)
	}
}

func TestEndingEverySessionLeavesNone(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	sess := session(u, "hash")
	_ = s.CreateSession(ctx, sess)
	if ended, err := s.DeleteOtherSessions(ctx, u.ID, nil); err != nil || len(ended) != 1 || ended[0] != sess.ID {
		t.Fatalf("ended %v, %v", ended, err)
	}
	if _, err := s.SessionByHash(ctx, sess.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a session survived: %v", err)
	}
}

func TestDeletingAUserTakesTheirSessions(t *testing.T) {
	ctx := context.Background()
	s, pool, u := stored(t)
	sess := session(u, "hash")
	_ = s.CreateSession(ctx, sess)
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, sess.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestAChallengeAllowsItsAttempts(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	c := app.Challenge{TokenHash: []byte("challenge"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0}
	_ = s.SaveChallenge(ctx, c, t0.Add(-15*time.Minute))
	for i := range 5 {
		if id, err := s.SpendChallengeAttempt(ctx, c.TokenHash, 5, t0); err != nil || id != u.ID {
			t.Fatalf("attempt %d = %v, %v", i+1, id, err)
		}
	}
	if _, err := s.SpendChallengeAttempt(ctx, c.TokenHash, 5, t0); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a sixth attempt = %v", err)
	}
}

func TestANewSignInInsideTheWindowReplacesTheTokenAndKeepsTheSpentAttempts(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	c := app.Challenge{TokenHash: []byte("challenge"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0}
	_ = s.SaveChallenge(ctx, c, t0.Add(-15*time.Minute))
	for range 3 {
		_, _ = s.SpendChallengeAttempt(ctx, c.TokenHash, 5, t0)
	}
	next := app.Challenge{TokenHash: []byte("next"), UserID: u.ID, ExpiresAt: t0.Add(10 * time.Minute), WindowStartedAt: t0.Add(5 * time.Minute)}
	if err := s.SaveChallenge(ctx, next, t0.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SpendChallengeAttempt(ctx, c.TokenHash, 5, t0); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the replaced token = %v", err)
	}
	for range 2 {
		if _, err := s.SpendChallengeAttempt(ctx, next.TokenHash, 5, t0.Add(5*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SpendChallengeAttempt(ctx, next.TokenHash, 5, t0.Add(5*time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a sixth attempt across two sign-ins = %v", err)
	}
}

func TestASignInAfterTheWindowStartsWithEveryAttempt(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	c := app.Challenge{TokenHash: []byte("challenge"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0}
	_ = s.SaveChallenge(ctx, c, t0.Add(-15*time.Minute))
	for range 5 {
		_, _ = s.SpendChallengeAttempt(ctx, c.TokenHash, 5, t0)
	}
	fresh := app.Challenge{TokenHash: []byte("fresh"), UserID: u.ID, ExpiresAt: t0.Add(25 * time.Minute), WindowStartedAt: t0.Add(20 * time.Minute)}
	_ = s.SaveChallenge(ctx, fresh, t0.Add(5*time.Minute))
	for i := range 5 {
		if _, err := s.SpendChallengeAttempt(ctx, fresh.TokenHash, 5, t0.Add(20*time.Minute)); err != nil {
			t.Fatalf("attempt %d = %v", i+1, err)
		}
	}
}

func TestAnExpiredChallengeTakesNoAttempt(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	c := app.Challenge{TokenHash: []byte("challenge"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0}
	_ = s.SaveChallenge(ctx, c, t0.Add(-15*time.Minute))
	if _, err := s.SpendChallengeAttempt(ctx, c.TokenHash, 10, t0.Add(5*time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestADeletedChallengeTakesNoAttempt(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	c := app.Challenge{TokenHash: []byte("challenge"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0}
	_ = s.SaveChallenge(ctx, c, t0.Add(-15*time.Minute))
	if err := s.DeleteChallenge(ctx, c.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SpendChallengeAttempt(ctx, c.TokenHash, 10, t0); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestARestartedAuthenticatorEnrolmentKeepsOnlyTheLatestSecret(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	if _, err := s.TOTPEnrolment(ctx, u.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an enrolment before any = %v", err)
	}
	for _, sealed := range []string{"vault:v1:first", "vault:v1:second"} {
		_ = s.SaveTOTPEnrolment(ctx, app.TOTPEnrolment{UserID: u.ID, SealedSecret: sealed, ExpiresAt: t0})
	}
	if e, err := s.TOTPEnrolment(ctx, u.ID); err != nil || e.SealedSecret != "vault:v1:second" || !e.ExpiresAt.Equal(t0) {
		t.Fatalf("TOTPEnrolment = %+v, %v", e, err)
	}
	if err := s.DeleteTOTPEnrolment(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
}

func phoneFactor(u domain.User) app.TOTPFactor {
	label, _ := domain.NewFactorLabel("Phone")
	return app.TOTPFactor{ID: uuid.New(), UserID: u.ID, Label: label, SealedSecret: "vault:v1:secret", LastStep: 100, CreatedAt: t0}
}

func TestAnAuthenticatorSecretInClearIsNeverStored(t *testing.T) {
	s, _, u := stored(t)
	plain := phoneFactor(u)
	plain.SealedSecret = "JBSWY3DPEHPK3PXP"
	if err := s.CreateTOTPFactor(context.Background(), plain); err == nil {
		t.Fatal("a secret in clear was stored")
	}
}

func TestOfTwoSignInsRacingForOneCodeStepExactlyOneWins(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	f := phoneFactor(u)
	_ = s.CreateTOTPFactor(ctx, f)
	wins := make(chan bool, 2)
	for range 2 {
		go func() {
			ok, err := s.UseTOTPStep(ctx, f.ID, 101, t0.Add(time.Minute))
			wins <- ok && err == nil
		}()
	}
	if a, b := <-wins, <-wins; a == b {
		t.Fatalf("racing uses of one step: %v and %v", a, b)
	}
}

func TestAnAuthenticatorCodeOlderThanTheLastOneUsedIsRefused(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	f := phoneFactor(u)
	_ = s.CreateTOTPFactor(ctx, f)
	if ok, _ := s.UseTOTPStep(ctx, f.ID, 100, t0); ok {
		t.Fatal("the last step used was accepted again")
	}
}

func TestAnAuthenticatorReadsBackWithItsLastUse(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	f := phoneFactor(u)
	_ = s.CreateTOTPFactor(ctx, f)
	_, _ = s.UseTOTPStep(ctx, f.ID, 101, t0.Add(time.Minute))
	got, err := s.TOTPFactors(ctx, u.ID)
	if err != nil || len(got) != 1 || got[0].LastStep != 101 || got[0].Label != f.Label || got[0].LastUsedAt == nil ||
		!got[0].LastUsedAt.Equal(t0.Add(time.Minute)) || !got[0].CreatedAt.Equal(t0) {
		t.Fatalf("TOTPFactors = %+v, %v", got, err)
	}
}

func TestOnlyItsOwnerRemovesAnAuthenticator(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	f := phoneFactor(u)
	_ = s.CreateTOTPFactor(ctx, f)
	if had, err := s.DeleteTOTPFactor(ctx, f.ID, uuid.New()); err != nil || had {
		t.Fatalf("another User removed it: %v, %v", had, err)
	}
	if had, err := s.DeleteTOTPFactor(ctx, f.ID, u.ID); err != nil || !had {
		t.Fatalf("its owner = %v, %v", had, err)
	}
}

func TestAnAuthenticatorLabelTheDomainRefusesIsAnError(t *testing.T) {
	ctx := context.Background()
	s, pool, u := stored(t)
	f := phoneFactor(u)
	_ = s.CreateTOTPFactor(ctx, f)
	if _, err := pool.Exec(ctx, `ALTER TABLE totp_factors DROP CONSTRAINT totp_factors_label_length`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE totp_factors SET label = ' ' WHERE id = $1`, f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TOTPFactors(ctx, u.ID); !errors.Is(err, domain.ErrInvalidLabel) {
		t.Fatalf("err = %v", err)
	}
}

func TestARestartedPasskeyEnrolmentKeepsOnlyTheLatestCeremony(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	if _, err := s.PasskeyEnrolment(ctx, u.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an enrolment before any = %v", err)
	}
	for _, c := range []string{"first", "second"} {
		_ = s.SavePasskeyEnrolment(ctx, app.PasskeyEnrolment{UserID: u.ID, Ceremony: []byte(c), ExpiresAt: t0})
	}
	if e, err := s.PasskeyEnrolment(ctx, u.ID); err != nil || string(e.Ceremony) != "second" || !e.ExpiresAt.Equal(t0) {
		t.Fatalf("PasskeyEnrolment = %+v, %v", e, err)
	}
	if err := s.DeletePasskeyEnrolment(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
}

func macBook(u domain.User) app.Passkey {
	label, _ := domain.NewFactorLabel("MacBook")
	return app.Passkey{ID: uuid.New(), UserID: u.ID, Label: label, CredentialID: []byte("cred"), Credential: []byte(`{"count": 0}`), CreatedAt: t0}
}

func TestOneCredentialIsOnePasskey(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	p := macBook(u)
	_ = s.CreatePasskey(ctx, p)
	twin := p
	twin.ID = uuid.New()
	if err := s.CreatePasskey(ctx, twin); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("err = %v", err)
	}
}

func TestAUsedPasskeyKeepsItsNewRecordAndWhenItWasUsed(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	p := macBook(u)
	_ = s.CreatePasskey(ctx, p)
	if kept, err := s.UsePasskey(ctx, u.ID, app.NewPasskey{CredentialID: []byte("cred"), Credential: []byte(`{"count": 1}`)}, []byte(`{"count": 0}`), t0.Add(time.Minute)); !kept || err != nil {
		t.Fatalf("UsePasskey = %v, %v", kept, err)
	}
	got, err := s.Passkeys(ctx, u.ID)
	if err != nil || len(got) != 1 || got[0].Label != p.Label || string(got[0].CredentialID) != "cred" ||
		string(got[0].Credential) != `{"count": 1}` || got[0].LastUsedAt == nil || !got[0].LastUsedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("Passkeys = %+v, %v", got, err)
	}
}

func TestASignInCheckedAgainstAStalePasskeyRecordLoses(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	_ = s.CreatePasskey(ctx, macBook(u))
	use := app.NewPasskey{CredentialID: []byte("cred"), Credential: []byte(`{"count": 1}`)}
	_, _ = s.UsePasskey(ctx, u.ID, use, []byte(`{"count": 0}`), t0.Add(time.Minute))
	if kept, err := s.UsePasskey(ctx, u.ID, use, []byte(`{"count": 0}`), t0.Add(time.Minute)); kept || err != nil {
		t.Fatalf("a stale use = %v, %v", kept, err)
	}
}

func TestOnlyItsOwnerRemovesAPasskey(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	p := macBook(u)
	_ = s.CreatePasskey(ctx, p)
	if had, err := s.DeletePasskey(ctx, p.ID, uuid.New()); err != nil || had {
		t.Fatalf("another User removed it: %v, %v", had, err)
	}
	if had, err := s.DeletePasskey(ctx, p.ID, u.ID); err != nil || !had {
		t.Fatalf("its owner = %v, %v", had, err)
	}
}

func signInChallenge(t *testing.T, s *persistence.Store, u domain.User) app.Challenge {
	t.Helper()
	c := app.Challenge{TokenHash: []byte("c1"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0}
	if err := s.SaveChallenge(context.Background(), c, t0.Add(-15*time.Minute)); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestASignInsPasskeyCeremonyLivesOnItsChallenge(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	c := signInChallenge(t, s, u)
	if err := s.SetChallengeCeremony(ctx, c.TokenHash, []byte("ceremony"), t0); err != nil {
		t.Fatal(err)
	}
	if id, ceremony, err := s.ChallengeCeremony(ctx, c.TokenHash, t0); err != nil || id != u.ID || string(ceremony) != "ceremony" {
		t.Fatalf("ChallengeCeremony = %v, %q, %v", id, ceremony, err)
	}
}

// Two requests answering one ceremony: the second waits on the first's lock, then finds it gone.
func TestAPasskeyCeremonyIsTakenOnceEvenByRacingRequests(t *testing.T) {
	ctx := context.Background()
	s, pool, u := stored(t)
	c := signInChallenge(t, s, u)
	_ = s.SetChallengeCeremony(ctx, c.TokenHash, []byte("ceremony"), t0)
	first, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queries.New(first).TakeChallengeCeremony(ctx, queries.TakeChallengeCeremonyParams{TokenHash: c.TokenHash, Now: t0}); err != nil {
		t.Fatal(err)
	}
	second := make(chan error, 1)
	go func() {
		_, _, err := s.TakeChallengeCeremony(ctx, c.TokenHash, t0)
		second <- err
	}()
	time.Sleep(200 * time.Millisecond) // the second request is now waiting on the lock
	if err := first.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-second; !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the waiting request = %v, want the ceremony gone", err)
	}
}

func TestATakenPasskeyCeremonyIsNotFoundAgain(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	c := signInChallenge(t, s, u)
	_ = s.SetChallengeCeremony(ctx, c.TokenHash, []byte("ceremony"), t0)
	_, _, _ = s.TakeChallengeCeremony(ctx, c.TokenHash, t0)
	if _, _, err := s.TakeChallengeCeremony(ctx, c.TokenHash, t0); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := s.TakeChallengeCeremony(ctx, []byte("none"), t0); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("no challenge = %v", err)
	}
}

func TestAnExpiredChallengeTakesNoPasskeyCeremony(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	c := signInChallenge(t, s, u)
	_ = s.SetChallengeCeremony(ctx, c.TokenHash, []byte("ceremony"), t0)
	if err := s.SetChallengeCeremony(ctx, c.TokenHash, []byte("late"), t0.Add(5*time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a late ceremony = %v", err)
	}
	if _, _, err := s.ChallengeCeremony(ctx, c.TokenHash, t0.Add(5*time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an expired challenge's ceremony = %v", err)
	}
}

func TestANewSignInClearsTheOldPasskeyCeremony(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	c := signInChallenge(t, s, u)
	_ = s.SetChallengeCeremony(ctx, c.TokenHash, []byte("ceremony"), t0)
	next := app.Challenge{TokenHash: []byte("c2"), UserID: u.ID, ExpiresAt: t0.Add(10 * time.Minute), WindowStartedAt: t0.Add(5 * time.Minute)}
	_ = s.SaveChallenge(ctx, next, t0.Add(-10*time.Minute))
	if _, ceremony, err := s.ChallengeCeremony(ctx, next.TokenHash, t0.Add(5*time.Minute)); err != nil || ceremony != nil {
		t.Fatalf("the new sign-in's ceremony = %q, %v", ceremony, err)
	}
}

func TestAPasskeyLabelTheDomainRefusesIsAnError(t *testing.T) {
	ctx := context.Background()
	s, pool, u := stored(t)
	p := macBook(u)
	_ = s.CreatePasskey(ctx, p)
	if _, err := pool.Exec(ctx, `ALTER TABLE passkeys DROP CONSTRAINT passkeys_label_length`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE passkeys SET label = ' ' WHERE id = $1`, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Passkeys(ctx, u.ID); !errors.Is(err, domain.ErrInvalidLabel) {
		t.Fatalf("err = %v", err)
	}
}

func TestFailuresOtherThanNotFoundPassThrough(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s, _ := open(t)
	cancel()
	e, _ := domain.NewEmail("ada@example.org")
	if _, err := s.UserByEmail(ctx, e); err == nil || errors.Is(err, app.ErrNotFound) || errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a cancelled query: %v", err)
	}
	if _, err := s.TOTPFactors(ctx, uuid.New()); err == nil {
		t.Fatal("a cancelled factor list")
	}
	if _, err := s.Passkeys(ctx, uuid.New()); err == nil {
		t.Fatal("a cancelled passkey list")
	}
	if _, _, err := s.TakeChallengeCeremony(ctx, []byte("x"), time.Now()); err == nil || errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a cancelled take = %v", err)
	}
}

func TestAUsersLiveSessionsAreListedTheMostRecentlySeenFirst(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	old, recent, gone := session(u, "old"), session(u, "recent"), session(u, "gone")
	old.Device, recent.Device = "Firefox on macOS", "Safari on iOS"
	recent.CreatedAt = t0.Add(time.Minute)
	gone.ExpiresAt = t0
	for _, sess := range []app.Session{old, recent, gone} {
		_ = s.CreateSession(ctx, sess)
	}
	_ = s.TouchSession(ctx, recent.TokenHash, t0.Add(2*time.Minute), t0.Add(time.Hour))
	got, err := s.SessionsOf(ctx, u.ID, t0.Add(time.Second))
	if err != nil || len(got) != 2 || got[0].Device != "Safari on iOS" || got[1].Device != "Firefox on macOS" || !bytes.Equal(got[1].TokenHash, old.TokenHash) {
		t.Fatalf("SessionsOf = %+v, %v", got, err)
	}
}

func TestDeletingASessionSaysWhetherItWasTheUsers(t *testing.T) {
	ctx := context.Background()
	s, _, u := stored(t)
	sess := session(u, "hash")
	_ = s.CreateSession(ctx, sess)
	other, _ := s.DeleteSession(ctx, uuid.New(), sess.ID)
	theirs, err := s.DeleteSession(ctx, u.ID, sess.ID)
	if other || !theirs || err != nil {
		t.Fatalf("someone else's %v, theirs %v, %v", other, theirs, err)
	}
}
