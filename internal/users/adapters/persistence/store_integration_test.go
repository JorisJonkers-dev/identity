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

func TestUsers(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	u := user(t)
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(ctx, user(t)); !errors.Is(err, app.ErrEmailTaken) {
		t.Fatalf("a second user with the same email: %v", err)
	}

	byEmail, err := s.UserByEmail(ctx, u.Email)
	if err != nil || byEmail.ID != u.ID || !byEmail.CreatedAt.Equal(t0) {
		t.Fatalf("UserByEmail = %+v, %v", byEmail, err)
	}
	missing, _ := domain.NewEmail("nobody@example.org")
	if _, err := s.UserByEmail(ctx, missing); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown email: %v", err)
	}
	if _, err := s.UserByID(ctx, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown id: %v", err)
	}

	if all, err := s.Users(ctx); err != nil || len(all) != 1 || all[0].ID != u.ID {
		t.Fatalf("Users = %+v, %v", all, err)
	}

	newName, _ := domain.NewName("Ada King")
	if err := s.UpdateName(ctx, u.ID, newName); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdatePasswordHash(ctx, u.ID, "stored-hash-2"); err != nil {
		t.Fatal(err)
	}
	got, err := s.UserByID(ctx, u.ID)
	if err != nil || got.Name != newName || got.PasswordHash != "stored-hash-2" {
		t.Fatalf("after the changes: %+v, %v", got, err)
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

func TestSignups(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	u := user(t)
	su := app.Signup{TokenHash: []byte("hash-of-a-token"), Email: u.Email, Name: u.Name, PasswordHash: "stored-hash-1", ExpiresAt: t0.Add(24 * time.Hour)}
	if err := s.CreateSignup(ctx, su); err != nil {
		t.Fatal(err)
	}
	got, err := s.SignupByHash(ctx, su.TokenHash)
	if err != nil || got.Email != su.Email || got.Name != su.Name || got.PasswordHash != su.PasswordHash || !got.ExpiresAt.Equal(su.ExpiresAt) {
		t.Fatalf("SignupByHash = %+v, %v", got, err)
	}
	if err := s.DeleteSignup(ctx, su.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SignupByHash(ctx, su.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("after deleting: %v", err)
	}

	for _, row := range [][2]string{{"not-an-email", "Ada"}, {"ada@example.org", ""}} {
		hash := []byte("bad-" + row[0] + row[1])
		if _, err := pool.Exec(ctx, `INSERT INTO signups (token_hash, email, name, password_hash, expires_at) VALUES ($1, $2, $3, 'h', now())`, hash, row[0], row[1]); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SignupByHash(ctx, hash); err == nil {
			t.Fatalf("a sign-up row %v the domain refuses was read", row)
		}
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	u := user(t)
	_ = s.CreateUser(ctx, u)
	sess := app.Session{TokenHash: []byte("hash"), ID: uuid.New(), UserID: u.ID, CreatedAt: t0, LastSeenAt: t0, ExpiresAt: t0.Add(time.Hour), Methods: []string{"pwd"}}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if err := s.TouchSession(ctx, sess.TokenHash, t0.Add(time.Minute), t0.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	got, err := s.SessionByHash(ctx, sess.TokenHash)
	if err != nil || !bytes.Equal(got.TokenHash, sess.TokenHash) || got.ID != sess.ID || got.UserID != u.ID ||
		!got.CreatedAt.Equal(t0) || !got.LastSeenAt.Equal(t0.Add(time.Minute)) || !got.ExpiresAt.Equal(t0.Add(2*time.Hour)) ||
		!slices.Equal(got.Methods, []string{"pwd"}) {
		t.Fatalf("SessionByHash = %+v, %v", got, err)
	}
	if err := s.SetSessionMethods(ctx, sess.TokenHash, []string{"pwd", "otp"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.SessionByHash(ctx, sess.TokenHash); !slices.Equal(got.Methods, []string{"pwd", "otp"}) {
		t.Fatalf("methods after a Second Factor = %v", got.Methods)
	}
	if err := s.DeleteSession(ctx, sess.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, sess.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("after deleting: %v", err)
	}
	if err := s.DeleteSession(ctx, sess.TokenHash); err != nil {
		t.Fatalf("deleting a session that does not exist: %v", err)
	}

	keep := app.Session{TokenHash: []byte("keep"), ID: uuid.New(), UserID: u.ID, CreatedAt: t0, LastSeenAt: t0, ExpiresAt: t0.Add(time.Hour), Methods: []string{"pwd"}}
	other := keep
	other.TokenHash, other.ID = []byte("other"), uuid.New()
	_ = s.CreateSession(ctx, keep)
	_ = s.CreateSession(ctx, other)
	if err := s.DeleteOtherSessions(ctx, u.ID, keep.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, other.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the other session survived: %v", err)
	}
	if _, err := s.SessionByHash(ctx, keep.TokenHash); err != nil {
		t.Fatalf("the kept session ended: %v", err)
	}

	if err := s.DeleteSessionsOfUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, keep.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a session survived signing out everywhere: %v", err)
	}

	// Deleting a User takes their sessions with them.
	_ = s.CreateSession(ctx, sess)
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionByHash(ctx, sess.TokenHash); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a deleted User's session: %v", err)
	}
}

func TestChallenges(t *testing.T) {
	ctx := context.Background()
	s, _ := open(t)
	u := user(t)
	_ = s.CreateUser(ctx, u)
	c := app.Challenge{TokenHash: []byte("challenge"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0}
	if err := s.SaveChallenge(ctx, c, t0.Add(-15*time.Minute)); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if id, err := s.SpendChallengeAttempt(ctx, c.TokenHash, 5, t0); err != nil || id != u.ID {
			t.Fatalf("attempt %d = %v, %v", i+1, id, err)
		}
	}

	// A new sign-in inside the window replaces the token and keeps the three spent attempts.
	next := app.Challenge{TokenHash: []byte("next"), UserID: u.ID, ExpiresAt: t0.Add(10 * time.Minute), WindowStartedAt: t0.Add(5 * time.Minute)}
	if err := s.SaveChallenge(ctx, next, t0.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SpendChallengeAttempt(ctx, c.TokenHash, 5, t0); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("the replaced token = %v", err)
	}
	for i := range 2 {
		if _, err := s.SpendChallengeAttempt(ctx, next.TokenHash, 5, t0.Add(5*time.Minute)); err != nil {
			t.Fatalf("attempt %d of the carried five = %v", i+4, err)
		}
	}
	if _, err := s.SpendChallengeAttempt(ctx, next.TokenHash, 5, t0.Add(5*time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a sixth attempt across two sign-ins = %v", err)
	}

	// Once the window has passed, a new sign-in starts with every attempt.
	fresh := app.Challenge{TokenHash: []byte("fresh"), UserID: u.ID, ExpiresAt: t0.Add(25 * time.Minute), WindowStartedAt: t0.Add(20 * time.Minute)}
	if err := s.SaveChallenge(ctx, fresh, t0.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if _, err := s.SpendChallengeAttempt(ctx, fresh.TokenHash, 5, t0.Add(20*time.Minute)); err != nil {
			t.Fatalf("attempt %d after the window = %v", i+1, err)
		}
	}
	if _, err := s.SpendChallengeAttempt(ctx, fresh.TokenHash, 10, t0.Add(25*time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an attempt after it expired = %v", err)
	}
	if err := s.DeleteChallenge(ctx, fresh.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SpendChallengeAttempt(ctx, fresh.TokenHash, 10, t0); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a deleted challenge = %v", err)
	}
}

func TestTOTPFactorsAndEnrolments(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	u := user(t)
	_ = s.CreateUser(ctx, u)

	if _, err := s.TOTPEnrolment(ctx, u.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("no enrolment = %v", err)
	}
	for _, sealed := range []string{"vault:v1:first", "vault:v1:second"} {
		if err := s.SaveTOTPEnrolment(ctx, app.TOTPEnrolment{UserID: u.ID, SealedSecret: sealed, ExpiresAt: t0}); err != nil {
			t.Fatal(err)
		}
	}
	if e, err := s.TOTPEnrolment(ctx, u.ID); err != nil || e.SealedSecret != "vault:v1:second" || !e.ExpiresAt.Equal(t0) {
		t.Fatalf("a restarted enrolment = %+v, %v", e, err)
	}
	if err := s.DeleteTOTPEnrolment(ctx, u.ID); err != nil {
		t.Fatal(err)
	}

	label, _ := domain.NewFactorLabel("Phone")
	f := app.TOTPFactor{ID: uuid.New(), UserID: u.ID, Label: label, SealedSecret: "vault:v1:secret", LastStep: 100, CreatedAt: t0}
	if err := s.CreateTOTPFactor(ctx, f); err != nil {
		t.Fatal(err)
	}
	plain := f
	plain.ID, plain.SealedSecret = uuid.New(), "JBSWY3DPEHPK3PXP"
	if err := s.CreateTOTPFactor(ctx, plain); err == nil {
		t.Fatal("a secret in clear was stored")
	}

	// Two sign-ins racing to accept step 101: exactly one wins.
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
	if ok, _ := s.UseTOTPStep(ctx, f.ID, 100, t0); ok {
		t.Fatal("an older step was accepted")
	}
	got, err := s.TOTPFactors(ctx, u.ID)
	if err != nil || len(got) != 1 || got[0].LastStep != 101 || got[0].Label != label || got[0].LastUsedAt == nil ||
		!got[0].LastUsedAt.Equal(t0.Add(time.Minute)) || !got[0].CreatedAt.Equal(t0) {
		t.Fatalf("TOTPFactors = %+v, %v", got, err)
	}

	if had, err := s.DeleteTOTPFactor(ctx, f.ID, uuid.New()); err != nil || had {
		t.Fatalf("deleting another User's factor = %v, %v", had, err)
	}
	if had, err := s.DeleteTOTPFactor(ctx, f.ID, u.ID); err != nil || !had {
		t.Fatalf("DeleteTOTPFactor = %v, %v", had, err)
	}

	// A label the domain refuses, written behind its back, is an error, not a factor.
	_ = s.CreateTOTPFactor(ctx, f)
	if _, err := pool.Exec(ctx, `ALTER TABLE totp_factors DROP CONSTRAINT totp_factors_label_length`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE totp_factors SET label = ' ' WHERE id = $1`, f.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TOTPFactors(ctx, u.ID); !errors.Is(err, domain.ErrInvalidLabel) {
		t.Fatalf("a bad label = %v", err)
	}
}

func TestPasskeys(t *testing.T) {
	ctx := context.Background()
	s, pool := open(t)
	u := user(t)
	_ = s.CreateUser(ctx, u)

	if _, err := s.PasskeyEnrolment(ctx, u.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("no enrolment = %v", err)
	}
	for _, c := range []string{"first", "second"} {
		if err := s.SavePasskeyEnrolment(ctx, app.PasskeyEnrolment{UserID: u.ID, Ceremony: []byte(c), ExpiresAt: t0}); err != nil {
			t.Fatal(err)
		}
	}
	if e, err := s.PasskeyEnrolment(ctx, u.ID); err != nil || string(e.Ceremony) != "second" || !e.ExpiresAt.Equal(t0) {
		t.Fatalf("a restarted enrolment = %+v, %v", e, err)
	}
	if err := s.DeletePasskeyEnrolment(ctx, u.ID); err != nil {
		t.Fatal(err)
	}

	label, _ := domain.NewFactorLabel("MacBook")
	p := app.Passkey{ID: uuid.New(), UserID: u.ID, Label: label, CredentialID: []byte("cred"), Credential: []byte(`{"count": 0}`), CreatedAt: t0}
	if err := s.CreatePasskey(ctx, p); err != nil {
		t.Fatal(err)
	}
	twin := p
	twin.ID = uuid.New()
	if err := s.CreatePasskey(ctx, twin); !errors.Is(err, app.ErrInvalidPasskey) {
		t.Fatalf("a credential id registered twice = %v", err)
	}
	if kept, err := s.UsePasskey(ctx, u.ID, app.NewPasskey{CredentialID: []byte("cred"), Credential: []byte(`{"count": 1}`)}, []byte(`{"count": 0}`), t0.Add(time.Minute)); !kept || err != nil {
		t.Fatalf("UsePasskey = %v, %v", kept, err)
	}
	// A second sign-in checked against the old record loses.
	if kept, err := s.UsePasskey(ctx, u.ID, app.NewPasskey{CredentialID: []byte("cred"), Credential: []byte(`{"count": 1}`)}, []byte(`{"count": 0}`), t0.Add(time.Minute)); kept || err != nil {
		t.Fatalf("a stale UsePasskey = %v, %v", kept, err)
	}
	got, err := s.Passkeys(ctx, u.ID)
	if err != nil || len(got) != 1 || got[0].Label != label || string(got[0].CredentialID) != "cred" ||
		string(got[0].Credential) != `{"count": 1}` || got[0].LastUsedAt == nil || !got[0].LastUsedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("Passkeys = %+v, %v", got, err)
	}
	if had, err := s.DeletePasskey(ctx, p.ID, uuid.New()); err != nil || had {
		t.Fatalf("deleting another User's passkey = %v, %v", had, err)
	}
	if had, err := s.DeletePasskey(ctx, p.ID, u.ID); err != nil || !had {
		t.Fatalf("DeletePasskey = %v, %v", had, err)
	}

	// A sign-in's ceremony lives on its challenge, and a new sign-in clears it.
	c := app.Challenge{TokenHash: []byte("c1"), UserID: u.ID, ExpiresAt: t0.Add(5 * time.Minute), WindowStartedAt: t0}
	_ = s.SaveChallenge(ctx, c, t0.Add(-15*time.Minute))
	if err := s.SetChallengeCeremony(ctx, c.TokenHash, []byte("ceremony"), t0); err != nil {
		t.Fatal(err)
	}
	if id, ceremony, err := s.ChallengeCeremony(ctx, c.TokenHash, t0); err != nil || id != u.ID || string(ceremony) != "ceremony" {
		t.Fatalf("ChallengeCeremony = %v, %q, %v", id, ceremony, err)
	}
	// A second request waits on the first one's lock, then finds the ceremony gone.
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
	if _, _, err := s.TakeChallengeCeremony(ctx, c.TokenHash, t0); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a taken ceremony taken again = %v", err)
	}
	if _, _, err := s.TakeChallengeCeremony(ctx, []byte("none"), t0); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("no challenge = %v", err)
	}
	if err := s.SetChallengeCeremony(ctx, c.TokenHash, []byte("ceremony"), t0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetChallengeCeremony(ctx, c.TokenHash, []byte("late"), t0.Add(5*time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a ceremony for an expired challenge = %v", err)
	}
	if _, _, err := s.ChallengeCeremony(ctx, c.TokenHash, t0.Add(5*time.Minute)); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an expired challenge's ceremony = %v", err)
	}
	next := app.Challenge{TokenHash: []byte("c2"), UserID: u.ID, ExpiresAt: t0.Add(10 * time.Minute), WindowStartedAt: t0.Add(5 * time.Minute)}
	_ = s.SaveChallenge(ctx, next, t0.Add(-10*time.Minute))
	if _, ceremony, err := s.ChallengeCeremony(ctx, next.TokenHash, t0.Add(5*time.Minute)); err != nil || ceremony != nil {
		t.Fatalf("a new sign-in kept the old ceremony: %q, %v", ceremony, err)
	}

	// A label the domain refuses, written behind its back, is an error, not a passkey.
	_ = s.CreatePasskey(ctx, p)
	if _, err := pool.Exec(ctx, `ALTER TABLE passkeys DROP CONSTRAINT passkeys_label_length`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE passkeys SET label = ' ' WHERE id = $1`, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Passkeys(ctx, u.ID); !errors.Is(err, domain.ErrInvalidLabel) {
		t.Fatalf("a bad label = %v", err)
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
