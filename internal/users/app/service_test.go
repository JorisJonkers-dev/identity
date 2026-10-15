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
	users    map[uuid.UUID]domain.User
	signups  map[string]app.Signup
	sessions map[string]app.Session
	fail     map[string]error
	touches  int
}

func newStore() *store {
	return &store{
		users:    map[uuid.UUID]domain.User{},
		signups:  map[string]app.Signup{},
		sessions: map[string]app.Session{},
		fail:     map[string]error{},
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

func (s *store) CreateSignup(_ context.Context, su app.Signup) error {
	if err := s.fail["CreateSignup"]; err != nil {
		return err
	}
	s.signups[string(su.TokenHash)] = su
	return nil
}

func (s *store) SignupByHash(_ context.Context, hash []byte) (app.Signup, error) {
	if err := s.fail["SignupByHash"]; err != nil {
		return app.Signup{}, err
	}
	su, ok := s.signups[string(hash)]
	if !ok {
		return app.Signup{}, app.ErrNotFound
	}
	return su, nil
}

func (s *store) DeleteSignup(_ context.Context, hash []byte) error {
	if err := s.fail["DeleteSignup"]; err != nil {
		return err
	}
	delete(s.signups, string(hash))
	return nil
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

func (s *store) DeleteOtherSessions(_ context.Context, userID uuid.UUID, keep []byte) error {
	if err := s.fail["DeleteOtherSessions"]; err != nil {
		return err
	}
	for k, sess := range s.sessions {
		if sess.UserID == userID && k != string(keep) {
			delete(s.sessions, k)
		}
	}
	return nil
}

// hasher "hashes" by prefixing, so a test can read what was stored.
type hasher struct {
	fail     error
	failHash error
	verified int
	hashed   int
}

func (h *hasher) Hash(p domain.Password) (string, error) {
	h.hashed++
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
	if err := f.Confirm(ctx, f.confirmationToken(t), pw); err != nil {
		t.Fatal(err)
	}
	e, _ := domain.NewEmail(email)
	u, err := f.store.UserByEmail(ctx, e)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

const pw = "correct horse battery"

func TestSignUpWaitsForConfirmationAndMailsALinkWithNoChosenText(t *testing.T) {
	f := newFixture()
	if err := f.SignUp(context.Background(), " Ada@Example.org ", "Ada <a href=evil>", pw); err != nil {
		t.Fatal(err)
	}
	if len(f.store.users) != 0 {
		t.Fatal("a User exists before their email is confirmed")
	}
	mail := f.mailer.sent[0]
	if mail.To != "ada@example.org" || mail.Subject != "Confirm your email" || strings.Contains(mail.Text, "Ada") || strings.Contains(mail.Text, "evil") {
		t.Fatalf("mail %+v carries text the signer-up chose", mail)
	}
	sum := sha256.Sum256([]byte(f.confirmationToken(t)))
	su, ok := f.store.signups[string(sum[:])]
	if !ok || su.Email.String() != "ada@example.org" || su.PasswordHash != "hash:"+pw || !su.ExpiresAt.Equal(t0.Add(24*time.Hour)) {
		t.Fatalf("sign-up %+v stored under the token's hash: %v", su, ok)
	}
}

func TestConfirmCreatesTheUserTheSignupDescribes(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_ = f.SignUp(ctx, "ada@example.org", " Ada ", pw)
	token := f.confirmationToken(t)
	f.now = t0.Add(time.Hour)
	if err := f.Confirm(ctx, token, pw); err != nil {
		t.Fatal(err)
	}
	e, _ := domain.NewEmail("ada@example.org")
	u, err := f.store.UserByEmail(ctx, e)
	if err != nil || u.Name.String() != "Ada" || u.PasswordHash != "hash:"+pw || !u.CreatedAt.Equal(t0.Add(time.Hour)) {
		t.Fatalf("created %+v, %v", u, err)
	}
	if err := f.Confirm(ctx, token, pw); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("a second use: %v", err)
	}
}

// An attacker signs up with the victim's email and their own password. The link goes to the
// victim; whichever link the victim follows, it confirms only with the password the victim knows,
// so the attacker's sign-up can never become the victim's account.
func TestASignupBeforeTheOwnersCannotTakeTheAccountOver(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_ = f.SignUp(ctx, "victim@example.org", "Attacker", "attacker's password")
	attackers := f.confirmationToken(t)
	_ = f.SignUp(ctx, "victim@example.org", "Victim", pw)
	victims := f.confirmationToken(t)

	if err := f.Confirm(ctx, attackers, pw); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("the victim following the attacker's link with their own password: %v", err)
	}
	if err := f.Confirm(ctx, victims, pw); err != nil {
		t.Fatal(err)
	}
	if _, err := f.SignIn(ctx, "victim@example.org", "attacker's password"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("the attacker's password signs in: %v", err)
	}
	if _, err := f.SignIn(ctx, "victim@example.org", pw); err != nil {
		t.Fatalf("the victim's password does not: %v", err)
	}
	if err := f.Confirm(ctx, attackers, "attacker's password"); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("the attacker's link after the victim confirmed: %v", err)
	}
}

func TestAWrongPasswordLeavesTheLinkUsable(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_ = f.SignUp(ctx, "ada@example.org", "Ada", pw)
	token := f.confirmationToken(t)
	if err := f.Confirm(ctx, token, "a typo in it!!!"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if err := f.Confirm(ctx, token, pw); err != nil {
		t.Fatalf("the right password after a typo: %v", err)
	}
}

func TestASecondConfirmationForAConfirmedEmailIsRefused(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_ = f.SignUp(ctx, "ada@example.org", "Ada", pw)
	first := f.confirmationToken(t)
	_ = f.SignUp(ctx, "ada@example.org", "Ada", "another password!")
	second := f.confirmationToken(t)
	_ = f.Confirm(ctx, first, pw)
	if err := f.Confirm(ctx, second, "another password!"); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("confirming a second sign-up for a confirmed email: %v", err)
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
	if len(f.store.signups) != 0 || len(f.mailer.sent) != 0 {
		t.Fatal("a refused sign-up stored or mailed something")
	}
}

// Hashing is the slow step, so a sign-up hashes whether or not the email is registered: the time
// an answer takes must not tell the two apart.
func TestSignUpCostsOneHashWhetherOrNotTheEmailIsRegistered(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	for _, email := range []string{"ada@example.org", "new@example.org"} {
		before := f.hasher.hashed
		if err := f.SignUp(context.Background(), email, "Ada", pw); err != nil {
			t.Fatal(err)
		}
		if f.hasher.hashed != before+1 {
			t.Errorf("%s: %d hashes, want 1", email, f.hasher.hashed-before)
		}
	}
}

func TestSignUpForARegisteredEmailSaysSoByMailOnly(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "ada@example.org")
	if err := f.SignUp(context.Background(), "ada@example.org", "Mallory", pw); err != nil {
		t.Fatalf("err = %v; a sign-up must not reveal that the email is registered", err)
	}
	last := f.mailer.sent[len(f.mailer.sent)-1]
	if last.Subject != "You already have an account" || !strings.Contains(last.Text, "https://auth.test/signin") || strings.Contains(last.Text, "Mallory") {
		t.Fatalf("mail %+v", last)
	}
}

func TestSignUpReportsStoreHasherAndMailerFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, method := range []string{"UserByEmail", "CreateSignup"} {
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
}

func TestConfirmIsSingleUseAndExpires(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	_ = f.SignUp(ctx, "late@example.org", "Late", pw)
	late := f.confirmationToken(t)
	f.now = t0.Add(25 * time.Hour)
	if err := f.Confirm(ctx, late, pw); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("expired token err = %v", err)
	}
	if len(f.store.signups) != 0 {
		t.Fatal("an expired sign-up was kept")
	}
	if err := f.Confirm(ctx, "never issued", pw); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("unknown token err = %v", err)
	}
}

func TestConfirmReportsStoreFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, method := range []string{"SignupByHash", "CreateUser", "DeleteSignup"} {
		f := newFixture()
		_ = f.SignUp(context.Background(), "a@b.c", "Ada", pw)
		token := f.confirmationToken(t)
		f.store.fail[method] = boom
		if err := f.Confirm(context.Background(), token, pw); !errors.Is(err, boom) {
			t.Errorf("%s failing: err = %v", method, err)
		}
	}
	f := newFixture()
	_ = f.SignUp(context.Background(), "a@b.c", "Ada", pw)
	token := f.confirmationToken(t)
	f.hasher.fail = boom
	if err := f.Confirm(context.Background(), token, pw); !errors.Is(err, boom) {
		t.Errorf("hasher failing: err = %v", err)
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

	if err := f.ChangePassword(ctx, id, "", "wrong password!!", "a new password!"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("wrong current password err = %v", err)
	}
	if err := f.ChangePassword(ctx, id, "", pw, "short"); !errors.Is(err, domain.ErrWeakPassword) {
		t.Fatalf("weak new password err = %v", err)
	}
	keep, _ := f.SignIn(ctx, "ada@example.org", pw)
	other, _ := f.SignIn(ctx, "ada@example.org", pw)
	if err := f.ChangePassword(ctx, id, keep, pw, "a new password!"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Authenticate(ctx, other); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("another session survived the password change: %v", err)
	}
	if _, err := f.Authenticate(ctx, keep); err != nil {
		t.Fatalf("the session that changed the password ended: %v", err)
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
	for _, method := range []string{"UpdateName", "UserByID", "UpdatePasswordHash", "DeleteOtherSessions"} {
		f := newFixture()
		id := f.signedUp(t, "a@b.c")
		f.store.fail[method] = boom
		var err error
		switch method {
		case "UpdateName":
			err = f.Rename(ctx, id, "Ada")
		default:
			err = f.ChangePassword(ctx, id, "", pw, "a new password!")
		}
		if !errors.Is(err, boom) {
			t.Errorf("%s failing: err = %v", method, err)
		}
	}
	f := newFixture()
	id := f.signedUp(t, "a@b.c")
	f.hasher.fail = boom
	if err := f.ChangePassword(ctx, id, "", pw, "a new password!"); !errors.Is(err, boom) {
		t.Errorf("hasher failing: err = %v", err)
	}
	f.hasher.fail, f.hasher.failHash = nil, boom
	if err := f.ChangePassword(ctx, id, "", pw, "a new password!"); !errors.Is(err, boom) {
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

func TestAFailedDecoyIsTriedAgainOnTheNextSignIn(t *testing.T) {
	f := newFixture()
	f.hasher.failHash = app.ErrBusy
	if _, err := f.SignIn(context.Background(), "nobody@b.c", pw); !errors.Is(err, app.ErrBusy) {
		t.Fatalf("first sign-in: %v", err)
	}
	f.hasher.failHash = nil
	if _, err := f.SignIn(context.Background(), "nobody@b.c", pw); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("a passing busy spell still breaks sign-in for unknown emails: %v", err)
	}
	hashed := f.hasher.hashed
	_, _ = f.SignIn(context.Background(), "nobody@b.c", pw)
	if f.hasher.hashed != hashed {
		t.Fatal("a decoy that was made is made again")
	}
}
