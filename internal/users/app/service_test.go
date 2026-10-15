package app_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// store is an in-memory app.Store; fail makes the named method return its error.
type store struct {
	users         map[uuid.UUID]domain.User
	confirmations map[string]app.Confirmation
	sessions      map[string]app.Session
	fail          map[string]error
	failOnce      map[string][]error
	touches       int
}

func newStore() *store {
	return &store{
		users:         map[uuid.UUID]domain.User{},
		confirmations: map[string]app.Confirmation{},
		sessions:      map[string]app.Session{},
		fail:          map[string]error{},
		failOnce:      map[string][]error{},
	}
}

func (s *store) CreateUser(_ context.Context, u domain.User) error {
	if err := s.fail["CreateUser"]; err != nil {
		return err
	}
	for _, other := range s.users {
		if other.Email == u.Email {
			return app.ErrEmailTaken
		}
	}
	s.users[u.ID] = u
	return nil
}

func (s *store) UserByEmail(_ context.Context, e domain.Email) (domain.User, error) {
	if err := s.fail["UserByEmail"]; err != nil {
		return domain.User{}, err
	}
	if queued := s.failOnce["UserByEmail"]; len(queued) > 0 {
		s.failOnce["UserByEmail"] = queued[1:]
		return domain.User{}, queued[0]
	}
	for _, u := range s.users {
		if u.Email == e {
			return u, nil
		}
	}
	return domain.User{}, app.ErrNotFound
}

func (s *store) UserByID(_ context.Context, id uuid.UUID) (domain.User, error) {
	if err := s.fail["UserByID"]; err != nil {
		return domain.User{}, err
	}
	u, ok := s.users[id]
	if !ok {
		return domain.User{}, app.ErrNotFound
	}
	return u, nil
}

func (s *store) ConfirmUser(_ context.Context, id uuid.UUID, at time.Time) error {
	if err := s.fail["ConfirmUser"]; err != nil {
		return err
	}
	u := s.users[id]
	u.ConfirmedAt = &at
	s.users[id] = u
	return nil
}

func (s *store) UpdateName(_ context.Context, id uuid.UUID, n domain.Name) error {
	if err := s.fail["UpdateName"]; err != nil {
		return err
	}
	u := s.users[id]
	u.Name = n
	s.users[id] = u
	return nil
}

func (s *store) UpdatePasswordHash(_ context.Context, id uuid.UUID, hash string) error {
	if err := s.fail["UpdatePasswordHash"]; err != nil {
		return err
	}
	u := s.users[id]
	u.PasswordHash = hash
	s.users[id] = u
	return nil
}

func (s *store) CreateConfirmation(_ context.Context, c app.Confirmation) error {
	if err := s.fail["CreateConfirmation"]; err != nil {
		return err
	}
	s.confirmations[string(c.TokenHash)] = c
	return nil
}

func (s *store) TakeConfirmation(_ context.Context, hash []byte) (app.Confirmation, error) {
	if err := s.fail["TakeConfirmation"]; err != nil {
		return app.Confirmation{}, err
	}
	c, ok := s.confirmations[string(hash)]
	if !ok {
		return app.Confirmation{}, app.ErrNotFound
	}
	delete(s.confirmations, string(hash))
	return c, nil
}

func (s *store) CreateSession(_ context.Context, sess app.Session) error {
	if err := s.fail["CreateSession"]; err != nil {
		return err
	}
	s.sessions[string(sess.TokenHash)] = sess
	return nil
}

func (s *store) SessionByHash(_ context.Context, hash []byte) (app.Session, error) {
	if err := s.fail["SessionByHash"]; err != nil {
		return app.Session{}, err
	}
	sess, ok := s.sessions[string(hash)]
	if !ok {
		return app.Session{}, app.ErrNotFound
	}
	return sess, nil
}

func (s *store) TouchSession(_ context.Context, hash []byte, lastSeen, expires time.Time) error {
	if err := s.fail["TouchSession"]; err != nil {
		return err
	}
	s.touches++
	sess := s.sessions[string(hash)]
	sess.LastSeenAt, sess.ExpiresAt = lastSeen, expires
	s.sessions[string(hash)] = sess
	return nil
}

func (s *store) DeleteSession(_ context.Context, hash []byte) error {
	if err := s.fail["DeleteSession"]; err != nil {
		return err
	}
	delete(s.sessions, string(hash))
	return nil
}

// hasher "hashes" by prefixing, so a test can read what was stored.
type hasher struct {
	fail     error
	failHash error
	verified int
}

func (h *hasher) Hash(p domain.Password) (string, error) {
	if h.failHash != nil {
		return "", h.failHash
	}
	return "hash:" + p.String(), h.fail
}

func (h *hasher) Verify(hash, password string) (bool, error) {
	h.verified++
	return hash == "hash:"+password, h.fail
}

type mailer struct {
	sent []app.Mail
	fail error
}

func (m *mailer) Send(_ context.Context, mail app.Mail) error {
	if m.fail != nil {
		return m.fail
	}
	m.sent = append(m.sent, mail)
	return nil
}

type fixture struct {
	*app.Service
	store  *store
	hasher *hasher
	mailer *mailer
	now    time.Time
}

func newFixture() *fixture {
	f := &fixture{store: newStore(), hasher: &hasher{}, mailer: &mailer{}, now: t0}
	// A reader that never runs out gives every token a distinct value.
	var n byte
	random := readerFunc(func(p []byte) (int, error) {
		n++
		for i := range p {
			p[i] = n
		}
		return len(p), nil
	})
	f.Service = app.New(f.store, f.hasher, f.mailer, func() time.Time { return f.now }, random, "https://auth.test")
	return f
}

type readerFunc func([]byte) (int, error)

func (r readerFunc) Read(p []byte) (int, error) { return r(p) }

// confirmationToken reads the token out of the last mail's link.
func (f *fixture) confirmationToken(t *testing.T) string {
	t.Helper()
	if len(f.mailer.sent) == 0 {
		t.Fatal("no mail was sent")
	}
	text := f.mailer.sent[len(f.mailer.sent)-1].Text
	_, after, ok := strings.Cut(text, "https://auth.test/confirm?token=")
	if !ok {
		t.Fatalf("no confirmation link in %q", text)
	}
	return strings.Fields(after)[0]
}

// signedUp signs a User up and confirms them, returning their id.
func (f *fixture) signedUp(t *testing.T, email string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	if err := f.SignUp(ctx, email, "Ada", pw); err != nil {
		t.Fatal(err)
	}
	if err := f.Confirm(ctx, f.confirmationToken(t)); err != nil {
		t.Fatal(err)
	}
	e, _ := domain.NewEmail(email)
	u, _ := f.store.UserByEmail(ctx, e)
	return u.ID
}

const pw = "correct horse battery"

func TestSignUpStoresAnUnconfirmedUserAndMailsALink(t *testing.T) {
	f := newFixture()
	if err := f.SignUp(context.Background(), " Ada@Example.org ", " Ada ", pw); err != nil {
		t.Fatal(err)
	}
	e, _ := domain.NewEmail("ada@example.org")
	u, err := f.store.UserByEmail(context.Background(), e)
	if err != nil || u.Name.String() != "Ada" || u.PasswordHash != "hash:"+pw || u.Confirmed() || !u.CreatedAt.Equal(t0) {
		t.Fatalf("stored %+v, %v", u, err)
	}
	mail := f.mailer.sent[0]
	if mail.To != "ada@example.org" || mail.Subject != "Confirm your email" {
		t.Fatalf("mail %+v", mail)
	}
	token := f.confirmationToken(t)
	sum := sha256.Sum256([]byte(token))
	c, ok := f.store.confirmations[string(sum[:])]
	if !ok || c.UserID != u.ID || !c.ExpiresAt.Equal(t0.Add(24*time.Hour)) {
		t.Fatalf("confirmation %+v stored under the token's hash: %v", c, ok)
	}
}

func TestSignUpRefusesWhatTheDomainRefuses(t *testing.T) {
	f := newFixture()
	cases := map[string]struct {
		email, name, password string
		want                  error
	}{
		"email":    {"nope", "Ada", pw, domain.ErrInvalidEmail},
		"name":     {"a@b.c", "", pw, domain.ErrInvalidName},
		"password": {"a@b.c", "Ada", "short", domain.ErrWeakPassword},
	}
	for name, c := range cases {
		if err := f.SignUp(context.Background(), c.email, c.name, c.password); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
	if len(f.store.users) != 0 || len(f.mailer.sent) != 0 {
		t.Fatal("a refused sign-up stored or mailed something")
	}
}

func TestSignUpAgainWhileUnconfirmedMailsANewLink(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_ = f.SignUp(ctx, "ada@example.org", "Ada", pw)
	first := f.confirmationToken(t)
	if err := f.SignUp(ctx, "ada@example.org", "Someone Else", "another password!"); err != nil {
		t.Fatal(err)
	}
	if second := f.confirmationToken(t); second == first || len(f.store.users) != 1 {
		t.Fatalf("second sign-up: token %q (first %q), %d users", second, first, len(f.store.users))
	}
	for _, u := range f.store.users {
		if u.Name.String() != "Ada" || u.PasswordHash != "hash:"+pw {
			t.Fatalf("a second sign-up changed the account: %+v", u)
		}
	}
}

func TestSignUpForAConfirmedEmailSaysSoByMailOnly(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	if err := f.SignUp(context.Background(), "ada@example.org", "Mallory", pw); err != nil {
		t.Fatalf("err = %v; a sign-up must not reveal that the email is registered", err)
	}
	last := f.mailer.sent[len(f.mailer.sent)-1]
	if last.Subject != "You already have an account" || !strings.Contains(last.Text, "https://auth.test/signin") {
		t.Fatalf("mail %+v", last)
	}
}

func TestSignUpRaceForTheSameEmailFallsBackToTheExistingUser(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_ = f.SignUp(ctx, "ada@example.org", "Ada", pw)
	first := f.confirmationToken(t)
	// The lookup misses, as if the other request had not committed yet, and the insert collides.
	f.store.failOnce["UserByEmail"] = []error{app.ErrNotFound}
	if err := f.SignUp(ctx, "ada@example.org", "Ada", pw); err != nil {
		t.Fatal(err)
	}
	if second := f.confirmationToken(t); second == first || len(f.store.users) != 1 {
		t.Fatalf("after the collision: token %q (first %q), %d users", second, first, len(f.store.users))
	}

	boom := errors.New("boom")
	f.store.failOnce["UserByEmail"] = []error{app.ErrNotFound, boom}
	if err := f.SignUp(ctx, "ada@example.org", "Ada", pw); !errors.Is(err, boom) {
		t.Fatalf("the lookup after a collision failing: err = %v", err)
	}
}

func TestSignUpReportsStoreHasherAndMailerFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, method := range []string{"UserByEmail", "CreateUser", "CreateConfirmation"} {
		f := newFixture()
		f.store.fail[method] = boom
		if err := f.SignUp(context.Background(), "a@b.c", "Ada", pw); !errors.Is(err, boom) {
			t.Errorf("%s failing: err = %v", method, err)
		}
	}
	f := newFixture()
	f.hasher.fail = boom
	if err := f.SignUp(context.Background(), "a@b.c", "Ada", pw); !errors.Is(err, boom) {
		t.Errorf("hasher failing: err = %v", err)
	}
	f = newFixture()
	f.mailer.fail = boom
	if err := f.SignUp(context.Background(), "a@b.c", "Ada", pw); !errors.Is(err, boom) {
		t.Errorf("mailer failing: err = %v", err)
	}
	f = newFixture()
	f.signedUp(t, "a@b.c")
	f.mailer.fail = boom
	if err := f.SignUp(context.Background(), "a@b.c", "Ada", pw); !errors.Is(err, boom) {
		t.Errorf("mailer failing for a confirmed email: err = %v", err)
	}
}

func TestConfirmIsSingleUseAndExpires(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_ = f.SignUp(ctx, "ada@example.org", "Ada", pw)
	token := f.confirmationToken(t)
	f.now = t0.Add(time.Hour)
	if err := f.Confirm(ctx, token); err != nil {
		t.Fatal(err)
	}
	for _, u := range f.store.users {
		if u.ConfirmedAt == nil || !u.ConfirmedAt.Equal(t0.Add(time.Hour)) {
			t.Fatalf("confirmed at %v", u.ConfirmedAt)
		}
	}
	if err := f.Confirm(ctx, token); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("second use err = %v", err)
	}

	_ = f.SignUp(ctx, "late@example.org", "Late", pw)
	late := f.confirmationToken(t)
	f.now = f.now.Add(25 * time.Hour)
	if err := f.Confirm(ctx, late); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("expired token err = %v", err)
	}
}

func TestConfirmReportsStoreFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, method := range []string{"TakeConfirmation", "ConfirmUser"} {
		f := newFixture()
		_ = f.SignUp(context.Background(), "a@b.c", "Ada", pw)
		token := f.confirmationToken(t)
		f.store.fail[method] = boom
		if err := f.Confirm(context.Background(), token); !errors.Is(err, boom) {
			t.Errorf("%s failing: err = %v", method, err)
		}
	}
}

func TestSignInStartsAThirtyDaySession(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	token, err := f.SignIn(context.Background(), "ADA@example.org", pw)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(token))
	sess, ok := f.store.sessions[string(sum[:])]
	if !ok || sess.UserID != id || !sess.ExpiresAt.Equal(t0.Add(30*24*time.Hour)) || !sess.CreatedAt.Equal(t0) {
		t.Fatalf("session %+v stored under the token's hash: %v", sess, ok)
	}
	if strings.Contains(string(sess.TokenHash), token) {
		t.Fatal("the session stores the token itself")
	}
}

func TestSignInRefusesWithOneAnswerAndOneCost(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	for name, c := range map[string][2]string{
		"wrong password": {"ada@example.org", "wrong password!!"},
		"unknown email":  {"bob@example.org", pw},
		"not an email":   {"bob", pw},
	} {
		before := f.hasher.verified
		if _, err := f.SignIn(context.Background(), c[0], c[1]); !errors.Is(err, app.ErrInvalidCredentials) {
			t.Errorf("%s: err = %v", name, err)
		}
		if name != "not an email" && f.hasher.verified != before+1 {
			t.Errorf("%s: %d hash checks, want 1, so timing does not tell the cases apart", name, f.hasher.verified-before)
		}
	}
}

func TestSignInBeforeConfirmingMailsANewLinkAndSaysSo(t *testing.T) {
	f := newFixture()
	_ = f.SignUp(context.Background(), "ada@example.org", "Ada", pw)
	sent := len(f.mailer.sent)
	if _, err := f.SignIn(context.Background(), "ada@example.org", pw); !errors.Is(err, app.ErrUnconfirmed) {
		t.Fatalf("err = %v", err)
	}
	if len(f.mailer.sent) != sent+1 || len(f.store.sessions) != 0 {
		t.Fatalf("%d new mails, %d sessions", len(f.mailer.sent)-sent, len(f.store.sessions))
	}
}

func TestSignInReportsFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, method := range []string{"UserByEmail", "CreateSession"} {
		f := newFixture()
		f.signedUp(t, "a@b.c")
		f.store.fail[method] = boom
		if _, err := f.SignIn(context.Background(), "a@b.c", pw); !errors.Is(err, boom) {
			t.Errorf("%s failing: err = %v", method, err)
		}
	}
	f := newFixture()
	f.signedUp(t, "a@b.c")
	f.hasher.fail = boom
	if _, err := f.SignIn(context.Background(), "a@b.c", pw); !errors.Is(err, boom) {
		t.Errorf("hasher failing: err = %v", err)
	}
	f = newFixture()
	f.hasher.fail = boom
	if _, err := f.SignIn(context.Background(), "nobody@b.c", pw); !errors.Is(err, boom) {
		t.Errorf("hasher failing on the decoy check: err = %v", err)
	}
	f = newFixture()
	_ = f.SignUp(context.Background(), "a@b.c", "Ada", pw)
	f.mailer.fail = boom
	if _, err := f.SignIn(context.Background(), "a@b.c", pw); !errors.Is(err, boom) {
		t.Errorf("mailer failing for an unconfirmed User: err = %v", err)
	}
}

func TestAuthenticateExtendsTheSessionAtMostOnceAMinute(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	token, _ := f.SignIn(context.Background(), "ada@example.org", pw)

	f.now = t0.Add(30 * time.Second)
	u, err := f.Authenticate(context.Background(), token)
	if err != nil || u.ID != id || f.store.touches != 0 {
		t.Fatalf("within a minute: %+v, %v, %d touches", u, err, f.store.touches)
	}
	f.now = t0.Add(2 * time.Minute)
	if _, err := f.Authenticate(context.Background(), token); err != nil || f.store.touches != 1 {
		t.Fatalf("after a minute: %v, %d touches", err, f.store.touches)
	}
	sum := sha256.Sum256([]byte(token))
	if got := f.store.sessions[string(sum[:])].ExpiresAt; !got.Equal(f.now.Add(30 * 24 * time.Hour)) {
		t.Fatalf("expires %v, want thirty days from the last use", got)
	}
}

func TestAuthenticateRefusesUnknownAndExpiredSessions(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	token, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	if _, err := f.Authenticate(context.Background(), "unknown"); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("unknown token err = %v", err)
	}
	f.now = t0.Add(31 * 24 * time.Hour)
	if _, err := f.Authenticate(context.Background(), token); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("expired session err = %v", err)
	}
}

func TestAuthenticateReportsStoreFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, method := range []string{"SessionByHash", "TouchSession", "UserByID"} {
		f := newFixture()
		f.signedUp(t, "a@b.c")
		token, _ := f.SignIn(context.Background(), "a@b.c", pw)
		f.now = t0.Add(time.Hour)
		f.store.fail[method] = boom
		if _, err := f.Authenticate(context.Background(), token); !errors.Is(err, boom) {
			t.Errorf("%s failing: err = %v", method, err)
		}
	}
}

func TestSignOutEndsTheSession(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	token, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	if err := f.SignOut(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Authenticate(context.Background(), token); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("after sign-out err = %v", err)
	}
}

func TestProfileChanges(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	ctx := context.Background()

	if err := f.Rename(ctx, id, "  Ada King "); err != nil || f.store.users[id].Name.String() != "Ada King" {
		t.Fatalf("rename: %v, %q", err, f.store.users[id].Name)
	}
	if err := f.Rename(ctx, id, ""); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("empty name err = %v", err)
	}

	if err := f.ChangePassword(ctx, id, "wrong password!!", "a new password!"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("wrong current password err = %v", err)
	}
	if err := f.ChangePassword(ctx, id, pw, "short"); !errors.Is(err, domain.ErrWeakPassword) {
		t.Fatalf("weak new password err = %v", err)
	}
	if err := f.ChangePassword(ctx, id, pw, "a new password!"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.SignIn(ctx, "ada@example.org", "a new password!"); err != nil {
		t.Fatalf("signing in with the new password: %v", err)
	}
	u, err := f.User(ctx, id)
	if err != nil || u.ID != id {
		t.Fatalf("User = %+v, %v", u, err)
	}
}

func TestProfileChangesReportFailures(t *testing.T) {
	boom := errors.New("boom")
	ctx := context.Background()
	for _, method := range []string{"UpdateName", "UserByID", "UpdatePasswordHash"} {
		f := newFixture()
		id := f.signedUp(t, "a@b.c")
		f.store.fail[method] = boom
		var err error
		switch method {
		case "UpdateName":
			err = f.Rename(ctx, id, "Ada")
		default:
			err = f.ChangePassword(ctx, id, pw, "a new password!")
		}
		if !errors.Is(err, boom) {
			t.Errorf("%s failing: err = %v", method, err)
		}
	}
	f := newFixture()
	id := f.signedUp(t, "a@b.c")
	f.hasher.fail = boom
	if err := f.ChangePassword(ctx, id, pw, "a new password!"); !errors.Is(err, boom) {
		t.Errorf("hasher failing: err = %v", err)
	}
	f.hasher.fail, f.hasher.failHash = nil, boom
	if err := f.ChangePassword(ctx, id, pw, "a new password!"); !errors.Is(err, boom) {
		t.Errorf("hashing the new password failing: err = %v", err)
	}
}

func TestTokensComeFromTheRandomSource(t *testing.T) {
	broken := readerFunc(func([]byte) (int, error) { return 0, errors.New("no entropy") })
	s := app.New(newStore(), &hasher{}, &mailer{}, time.Now, broken, "https://auth.test")
	if err := s.SignUp(context.Background(), "a@b.c", "Ada", pw); err == nil {
		t.Fatal("a sign-up without entropy succeeded")
	}

	f := newFixture()
	f.signedUp(t, "a@b.c")
	f.Service = app.New(f.store, f.hasher, f.mailer, func() time.Time { return f.now }, broken, "https://auth.test")
	if _, err := f.SignIn(context.Background(), "a@b.c", pw); err == nil {
		t.Fatal("a sign-in without entropy succeeded")
	}
}
