package app_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

// store is an in-memory app.Store; fail makes the named method return its error.
type store struct {
	users      map[uuid.UUID]domain.User
	signups    map[string]app.Signup
	sessions   map[string]app.Session
	challenges map[string]*challenge
	changes    map[uuid.UUID]app.EmailChange
	resets     map[uuid.UUID]app.PasswordReset
	// racedReset makes TakePasswordReset find the reset already taken.
	racedReset bool
	// tokens is each User's Personal Tokens, which go with them.
	tokens     map[uuid.UUID][]uuid.UUID
	factors    []app.TOTPFactor
	enrolments map[uuid.UUID]app.TOTPEnrolment
	passkeys   []app.Passkey
	pending    map[uuid.UUID]app.PasskeyEnrolment
	fail       map[string]error
	touches    int
	// stale makes UseTOTPStep lose to another sign-in that used the step first.
	stale bool
	// passkeyReads fails Passkeys from the read after this many, when set.
	passkeyReads *int
}

type challenge struct {
	app.Challenge
	attempts int
	ceremony []byte
}

func (s *store) Passkeys(_ context.Context, userID uuid.UUID) ([]app.Passkey, error) {
	if err := s.fail["Passkeys"]; err != nil {
		return nil, err
	}
	if s.passkeyReads != nil {
		if *s.passkeyReads == 0 {
			return nil, errors.New("passkeys unreadable")
		}
		*s.passkeyReads--
	}
	var out []app.Passkey
	for _, p := range s.passkeys {
		if p.UserID == userID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *store) CreatePasskey(_ context.Context, p app.Passkey) error {
	if err := s.fail["CreatePasskey"]; err != nil {
		return err
	}
	s.passkeys = append(s.passkeys, p)
	return nil
}

func (s *store) UsePasskey(_ context.Context, userID uuid.UUID, used app.NewPasskey, previous []byte, at time.Time) (bool, error) {
	if err := s.fail["UsePasskey"]; err != nil {
		return false, err
	}
	for i, p := range s.passkeys {
		if p.UserID == userID && string(p.CredentialID) == string(used.CredentialID) && string(p.Credential) == string(previous) {
			s.passkeys[i].Credential, s.passkeys[i].LastUsedAt = used.Credential, &at
			return true, nil
		}
	}
	return false, nil
}

func (s *store) TakeChallengeCeremony(ctx context.Context, hash []byte, now time.Time) (uuid.UUID, []byte, error) {
	if err := s.fail["TakeChallengeCeremony"]; err != nil {
		return uuid.Nil, nil, err
	}
	userID, ceremony, err := s.ChallengeCeremony(ctx, hash, now)
	if err != nil || ceremony == nil {
		return uuid.Nil, nil, app.ErrNotFound
	}
	s.challenges[string(hash)].ceremony = nil
	return userID, ceremony, nil
}

func (s *store) DeletePasskey(_ context.Context, id, userID uuid.UUID) (bool, error) {
	if err := s.fail["DeletePasskey"]; err != nil {
		return false, err
	}
	for i, p := range s.passkeys {
		if p.ID == id && p.UserID == userID {
			s.passkeys = append(s.passkeys[:i], s.passkeys[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (s *store) SavePasskeyEnrolment(_ context.Context, e app.PasskeyEnrolment) error {
	if err := s.fail["SavePasskeyEnrolment"]; err != nil {
		return err
	}
	s.pending[e.UserID] = e
	return nil
}

func (s *store) PasskeyEnrolment(_ context.Context, userID uuid.UUID) (app.PasskeyEnrolment, error) {
	if err := s.fail["PasskeyEnrolment"]; err != nil {
		return app.PasskeyEnrolment{}, err
	}
	e, ok := s.pending[userID]
	if !ok {
		return app.PasskeyEnrolment{}, app.ErrNotFound
	}
	return e, nil
}

func (s *store) DeletePasskeyEnrolment(_ context.Context, userID uuid.UUID) error {
	if err := s.fail["DeletePasskeyEnrolment"]; err != nil {
		return err
	}
	delete(s.pending, userID)
	return nil
}

func (s *store) ChallengeCeremony(_ context.Context, hash []byte, now time.Time) (uuid.UUID, []byte, error) {
	if err := s.fail["ChallengeCeremony"]; err != nil {
		return uuid.Nil, nil, err
	}
	c, ok := s.challenges[string(hash)]
	if !ok || !now.Before(c.ExpiresAt) {
		return uuid.Nil, nil, app.ErrNotFound
	}
	return c.UserID, c.ceremony, nil
}

func (s *store) SetChallengeCeremony(_ context.Context, hash, ceremony []byte, now time.Time) error {
	if err := s.fail["SetChallengeCeremony"]; err != nil {
		return err
	}
	c, ok := s.challenges[string(hash)]
	if !ok || !now.Before(c.ExpiresAt) {
		return app.ErrNotFound
	}
	c.ceremony = ceremony
	return nil
}

func newStore() *store {
	return &store{
		users:      map[uuid.UUID]domain.User{},
		signups:    map[string]app.Signup{},
		sessions:   map[string]app.Session{},
		challenges: map[string]*challenge{},
		changes:    map[uuid.UUID]app.EmailChange{},
		resets:     map[uuid.UUID]app.PasswordReset{},
		enrolments: map[uuid.UUID]app.TOTPEnrolment{},
		pending:    map[uuid.UUID]app.PasskeyEnrolment{},
		fail:       map[string]error{},
	}
}

func (s *store) SetSessionMethods(_ context.Context, hash []byte, methods []string, at time.Time) error {
	if err := s.fail["SetSessionMethods"]; err != nil {
		return err
	}
	sess := s.sessions[string(hash)]
	sess.Methods, sess.TwoFactorAt = methods, &at
	s.sessions[string(hash)] = sess
	return nil
}

func (s *store) StepUpChallenge(_ context.Context, sessionHash []byte) ([]byte, error) {
	if err := s.fail["StepUpChallenge"]; err != nil {
		return nil, err
	}
	for _, c := range s.challenges {
		if c.SessionHash != nil && string(c.SessionHash) == string(sessionHash) {
			return c.TokenHash, nil
		}
	}
	return nil, app.ErrNotFound
}

// SaveChallenge keeps one challenge per User and carries its attempts, as the SQL does.
func (s *store) SaveChallenge(_ context.Context, c app.Challenge, windowSince time.Time) error {
	if err := s.fail["SaveChallenge"]; err != nil {
		return err
	}
	for k, old := range s.challenges {
		if old.UserID != c.UserID {
			continue
		}
		delete(s.challenges, k)
		if old.WindowStartedAt.After(windowSince) {
			c.WindowStartedAt = old.WindowStartedAt
			s.challenges[string(c.TokenHash)] = &challenge{Challenge: c, attempts: old.attempts}
			return nil
		}
	}
	s.challenges[string(c.TokenHash)] = &challenge{Challenge: c}
	return nil
}

func (s *store) SpendChallengeAttempt(_ context.Context, hash []byte, maxAttempts int, now time.Time) (uuid.UUID, error) {
	if err := s.fail["SpendChallengeAttempt"]; err != nil {
		return uuid.Nil, err
	}
	c, ok := s.challenges[string(hash)]
	if !ok || c.attempts >= maxAttempts || !now.Before(c.ExpiresAt) {
		return uuid.Nil, app.ErrNotFound
	}
	c.attempts++
	return c.UserID, nil
}

func (s *store) DeleteChallenge(_ context.Context, hash []byte) error {
	if err := s.fail["DeleteChallenge"]; err != nil {
		return err
	}
	delete(s.challenges, string(hash))
	return nil
}

func (s *store) TOTPFactors(_ context.Context, userID uuid.UUID) ([]app.TOTPFactor, error) {
	if err := s.fail["TOTPFactors"]; err != nil {
		return nil, err
	}
	var out []app.TOTPFactor
	for _, f := range s.factors {
		if f.UserID == userID {
			out = append(out, f)
		}
	}
	return out, nil
}

func (s *store) CreateTOTPFactor(_ context.Context, f app.TOTPFactor) error {
	if err := s.fail["CreateTOTPFactor"]; err != nil {
		return err
	}
	s.factors = append(s.factors, f)
	return nil
}

func (s *store) UseTOTPStep(_ context.Context, id uuid.UUID, step int64, _ time.Time) (bool, error) {
	if err := s.fail["UseTOTPStep"]; err != nil {
		return false, err
	}
	for i, f := range s.factors {
		if f.ID == id && f.LastStep < step && !s.stale {
			s.factors[i].LastStep = step
			return true, nil
		}
	}
	return false, nil
}

func (s *store) DeleteTOTPFactor(_ context.Context, id, userID uuid.UUID) (bool, error) {
	if err := s.fail["DeleteTOTPFactor"]; err != nil {
		return false, err
	}
	for i, f := range s.factors {
		if f.ID == id && f.UserID == userID {
			s.factors = append(s.factors[:i], s.factors[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (s *store) SaveTOTPEnrolment(_ context.Context, e app.TOTPEnrolment) error {
	if err := s.fail["SaveTOTPEnrolment"]; err != nil {
		return err
	}
	s.enrolments[e.UserID] = e
	return nil
}

func (s *store) TOTPEnrolment(_ context.Context, userID uuid.UUID) (app.TOTPEnrolment, error) {
	if err := s.fail["TOTPEnrolment"]; err != nil {
		return app.TOTPEnrolment{}, err
	}
	e, ok := s.enrolments[userID]
	if !ok {
		return app.TOTPEnrolment{}, app.ErrNotFound
	}
	return e, nil
}

func (s *store) DeleteTOTPEnrolment(_ context.Context, userID uuid.UUID) error {
	if err := s.fail["DeleteTOTPEnrolment"]; err != nil {
		return err
	}
	delete(s.enrolments, userID)
	return nil
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

func (s *store) Users(context.Context) ([]domain.User, error) {
	if err := s.fail["Users"]; err != nil {
		return nil, err
	}
	users := make([]domain.User, 0, len(s.users))
	for _, u := range s.users {
		users = append(users, u)
	}
	return users, nil
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

func (s *store) SaveEmailChange(_ context.Context, c app.EmailChange) error {
	if err := s.fail["SaveEmailChange"]; err != nil {
		return err
	}
	s.changes[c.UserID] = c
	return nil
}

func (s *store) EmailChangeOf(_ context.Context, userID uuid.UUID) (app.EmailChange, error) {
	if err := s.fail["EmailChangeOf"]; err != nil {
		return app.EmailChange{}, err
	}
	c, ok := s.changes[userID]
	if !ok {
		return app.EmailChange{}, app.ErrNotFound
	}
	return c, nil
}

func (s *store) TakeEmailChange(_ context.Context, hash []byte) (app.EmailChange, error) {
	if err := s.fail["TakeEmailChange"]; err != nil {
		return app.EmailChange{}, err
	}
	for id, c := range s.changes {
		if string(c.TokenHash) == string(hash) {
			delete(s.changes, id)
			return c, nil
		}
	}
	return app.EmailChange{}, app.ErrNotFound
}

func (s *store) DeleteEmailChange(_ context.Context, userID uuid.UUID) error {
	delete(s.changes, userID)
	return s.fail["DeleteEmailChange"]
}

func (s *store) UpdateEmail(_ context.Context, id uuid.UUID, e domain.Email) error {
	if err := s.fail["UpdateEmail"]; err != nil {
		return err
	}
	for other, u := range s.users {
		if other != id && u.Email == e {
			return app.ErrEmailTaken
		}
	}
	u := s.users[id]
	u.Email = e
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

// CountActiveSessions counts the sessions unexpired at now, and the Users holding them.
func (s *store) CountActiveSessions(_ context.Context, now time.Time) (sessions, users int, err error) {
	holders := map[uuid.UUID]bool{}
	for _, sess := range s.sessions {
		if now.Before(sess.ExpiresAt) {
			sessions++
			holders[sess.UserID] = true
		}
	}
	return sessions, len(holders), s.fail["CountActiveSessions"]
}

func (s *store) SessionByID(_ context.Context, id uuid.UUID) (app.Session, error) {
	if err := s.fail["SessionByID"]; err != nil {
		return app.Session{}, err
	}
	for _, sess := range s.sessions {
		if sess.ID == id {
			return sess, nil
		}
	}
	return app.Session{}, app.ErrNotFound
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

func (s *store) SavePasswordReset(_ context.Context, r app.PasswordReset) error {
	if err := s.fail["SavePasswordReset"]; err != nil {
		return err
	}
	s.resets[r.UserID] = r
	return nil
}

func (s *store) PasswordResetByHash(_ context.Context, hash []byte) (app.PasswordReset, error) {
	if err := s.fail["PasswordResetByHash"]; err != nil {
		return app.PasswordReset{}, err
	}
	for _, r := range s.resets {
		if string(r.TokenHash) == string(hash) {
			return r, nil
		}
	}
	return app.PasswordReset{}, app.ErrNotFound
}

func (s *store) ProvePasswordReset(_ context.Context, userID uuid.UUID, at time.Time) error {
	if err := s.fail["ProvePasswordReset"]; err != nil {
		return err
	}
	r := s.resets[userID]
	r.ProvedAt = &at
	s.resets[userID] = r
	return nil
}

func (s *store) TakePasswordReset(_ context.Context, hash []byte) (bool, error) {
	if err := s.fail["TakePasswordReset"]; err != nil || s.racedReset {
		return false, err
	}
	for id, r := range s.resets {
		if string(r.TokenHash) == string(hash) {
			delete(s.resets, id)
			return true, nil
		}
	}
	return false, nil
}

func (s *store) UsersWithTwoFactor(context.Context) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	for _, f := range s.factors {
		ids = append(ids, f.UserID)
	}
	for _, k := range s.passkeys {
		ids = append(ids, k.UserID)
	}
	return ids, s.fail["UsersWithTwoFactor"]
}

func (s *store) DeleteUser(_ context.Context, userID uuid.UUID) (app.Deleted, error) {
	if err := s.fail["DeleteUser"]; err != nil {
		return app.Deleted{}, err
	}
	var d app.Deleted
	for k, sess := range s.sessions {
		if sess.UserID == userID {
			d.Sessions = append(d.Sessions, sess.ID)
			delete(s.sessions, k)
		}
	}
	d.Tokens = s.tokens[userID]
	delete(s.users, userID)
	return d, nil
}

func (s *store) SessionsOf(_ context.Context, userID uuid.UUID, now time.Time) ([]app.Session, error) {
	if err := s.fail["SessionsOf"]; err != nil {
		return nil, err
	}
	var out []app.Session
	for _, sess := range s.sessions {
		if sess.UserID == userID && now.Before(sess.ExpiresAt) {
			out = append(out, sess)
		}
	}
	slices.SortFunc(out, func(a, b app.Session) int { return b.LastSeenAt.Compare(a.LastSeenAt) })
	return out, nil
}

func (s *store) DeleteSession(_ context.Context, userID, id uuid.UUID) (bool, error) {
	if err := s.fail["DeleteSession"]; err != nil {
		return false, err
	}
	for k, sess := range s.sessions {
		if sess.UserID == userID && sess.ID == id {
			delete(s.sessions, k)
			return true, nil
		}
	}
	return false, nil
}

func (s *store) DeleteOtherSessions(_ context.Context, userID uuid.UUID, keep []byte) ([]uuid.UUID, error) {
	if err := s.fail["DeleteOtherSessions"]; err != nil {
		return nil, err
	}
	var ended []uuid.UUID
	for k, sess := range s.sessions {
		if sess.UserID == userID && (keep == nil || k != string(keep)) {
			ended = append(ended, sess.ID)
			delete(s.sessions, k)
		}
	}
	return ended, nil
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

// sealer "seals" by prefixing, as Vault's ciphertexts are, so a test can see nothing is in clear.
type sealer struct{ failSeal, failOpen error }

func (s *sealer) Seal(_ context.Context, p []byte) (string, error) {
	return "vault:v1:" + base64.StdEncoding.EncodeToString(p), s.failSeal
}

func (s *sealer) Open(_ context.Context, c string) ([]byte, error) {
	if s.failOpen != nil {
		return nil, s.failOpen
	}
	return base64.StdEncoding.DecodeString(strings.TrimPrefix(c, "vault:v1:"))
}

// webAuthn stands in for the ceremonies: a registration answer is "made:" plus its ceremony, and a
// login answer "signed:" plus its ceremony and the credential id it signs with.
type webAuthn struct {
	n         int
	failBegin error
	// staleUse runs after a login checks out, as a racing sign-in's write would.
	staleUse func()
}

func (w *webAuthn) BeginRegistration(u app.PasskeyUser) ([]byte, []byte, error) {
	return []byte(`{"publicKey":{}}`), []byte("reg-" + u.ID.String()), w.failBegin
}

func (w *webAuthn) FinishRegistration(_ app.PasskeyUser, ceremony, response []byte) (app.NewPasskey, error) {
	if string(response) != "made:"+string(ceremony) {
		return app.NewPasskey{}, errors.New("refused")
	}
	w.n++
	id := fmt.Sprintf("cred-%d", w.n)
	return app.NewPasskey{CredentialID: []byte(id), Credential: []byte(`{"id":"` + id + `","count":0}`)}, nil
}

func (w *webAuthn) BeginLogin(u app.PasskeyUser) ([]byte, []byte, error) {
	return []byte(`{"publicKey":{}}`), []byte("login-" + u.ID.String()), w.failBegin
}

func (w *webAuthn) FinishLogin(u app.PasskeyUser, ceremony, response []byte) (app.NewPasskey, error) {
	ceremonyPart, credID, _ := strings.Cut(strings.TrimPrefix(string(response), "signed:"), "|")
	if ceremonyPart != string(ceremony) {
		return app.NewPasskey{}, errors.New("refused")
	}
	if credID == "ghost" {
		return app.NewPasskey{CredentialID: []byte("ghost")}, nil
	}
	for _, c := range u.Credentials {
		if strings.Contains(string(c), `"`+credID+`"`) {
			if w.staleUse != nil {
				w.staleUse()
			}
			return app.NewPasskey{CredentialID: []byte(credID), Credential: []byte(`{"id":"` + credID + `","count":1}`)}, nil
		}
	}
	return app.NewPasskey{}, errors.New("unknown credential")
}

// admins names the Admins; err fails every check.
type admins struct {
	ids map[uuid.UUID]bool
	err error
}

func (a *admins) IsAdmin(_ context.Context, id uuid.UUID) (bool, error) { return a.ids[id], a.err }

type fixture struct {
	*app.Service
	store  *store
	hasher *hasher
	mailer *mailer
	sealer *sealer
	wa     *webAuthn
	admins *admins
	events *recorded
	now    time.Time
	// dry makes every read of randomness fail.
	dry bool
}

// newFixture is a Service over fakes, with opts after the usual ones.
func newFixture(opts ...app.Option) *fixture {
	f := &fixture{store: newStore(), hasher: &hasher{}, mailer: &mailer{}, sealer: &sealer{}, wa: &webAuthn{}, admins: &admins{ids: map[uuid.UUID]bool{}}, events: &recorded{}, now: t0}
	// A reader that never runs out gives every token a distinct value.
	var n byte
	random := readerFunc(func(p []byte) (int, error) {
		if f.dry {
			return 0, errors.New("no randomness")
		}
		n++
		for i := range p {
			p[i] = n
		}
		return len(p), nil
	})
	f.Service = app.New(f.store, f.hasher, f.mailer, func() time.Time { return f.now }, random, "https://auth.test",
		append([]app.Option{app.WithSealer(f.sealer), app.WithWebAuthn(f.wa), app.WithAdmins(f.admins), app.WithAudit(f.events)}, opts...)...)
	return f
}

// recorded is the Audit Events a Service recorded.
type recorded []audit.Event

func (r *recorded) Record(_ context.Context, e audit.Event) { *r = append(*r, e) }

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

func TestSignOutEndsEverySessionAndSaysWhich(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	var told []uuid.UUID
	f.Service = app.New(f.store, f.hasher, f.mailer, func() time.Time { return f.now }, readerFunc(func(p []byte) (int, error) {
		_, err := rand.Read(p)
		return len(p), err
	}), "https://auth.test", app.WithSignedOut(func(_ context.Context, _ uuid.UUID, ids []uuid.UUID) { told = append(told, ids...) }))
	f.signedUp(t, "ada@example.org")
	here, _ := f.SignIn(ctx, "ada@example.org", pw)
	elsewhere, _ := f.SignIn(ctx, "ada@example.org", pw)
	ids := []uuid.UUID{f.store.sessions[sum(here)].ID, f.store.sessions[sum(elsewhere)].ID}
	if err := f.SignOut(ctx, here); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{here, elsewhere} {
		if _, err := f.Authenticate(ctx, tok); !errors.Is(err, app.ErrNoSession) {
			t.Fatalf("a session survived signing out: %v", err)
		}
	}
	slices.SortFunc(told, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	if !slices.Equal(told, ids) {
		t.Fatalf("told %v, want %v", told, ids)
	}
	told = nil
	if err := f.SignOut(ctx, here); err != nil || told != nil {
		t.Fatalf("signing out a gone session = %v, told %v", err, told)
	}
	boom := errors.New("boom")
	f.store.fail["SessionByHash"] = boom
	if err := f.SignOut(ctx, here); !errors.Is(err, boom) {
		t.Fatalf("a failing lookup = %v", err)
	}
}

func TestAUserRenamesThemselfWithTheNameTrimmed(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if err := f.Rename(context.Background(), id, "  Ada King "); err != nil || f.store.users[id].Name.String() != "Ada King" {
		t.Fatalf("Rename = %v, name %q", err, f.store.users[id].Name)
	}
}

func TestANameCannotBeEmpty(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if err := f.Rename(context.Background(), id, ""); !errors.Is(err, domain.ErrInvalidName) {
		t.Fatalf("err = %v", err)
	}
}

func TestChangingThePasswordNeedsTheCurrentOne(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if err := f.ChangePassword(context.Background(), id, "", "wrong password!!", "a new password!"); !errors.Is(err, app.ErrInvalidCredentials) {
		t.Fatalf("err = %v", err)
	}
}

func TestANewPasswordMustBeStrong(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if err := f.ChangePassword(context.Background(), id, "", pw, "short"); !errors.Is(err, domain.ErrWeakPassword) {
		t.Fatalf("err = %v", err)
	}
}

func TestChangingThePasswordEndsEveryOtherSessionButThisOne(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	keep, _ := f.SignIn(ctx, "ada@example.org", pw)
	other, _ := f.SignIn(ctx, "ada@example.org", pw)
	if err := f.ChangePassword(ctx, id, keep, pw, "a new password!"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Authenticate(ctx, other); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("another session survived: %v", err)
	}
	if _, err := f.Authenticate(ctx, keep); err != nil {
		t.Fatalf("the session that changed the password ended: %v", err)
	}
}

func TestTheNewPasswordSignsIn(t *testing.T) {
	ctx := context.Background()
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	_ = f.ChangePassword(ctx, id, "", pw, "a new password!")
	if _, err := f.SignIn(ctx, "ada@example.org", "a new password!"); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestAUserIsReadByID(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	if u, err := f.User(context.Background(), id); err != nil || u.ID != id {
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

	// Entropy for the token but none for the session's id.
	reads := 0
	once := readerFunc(func(p []byte) (int, error) {
		reads++
		if reads > 1 {
			return 0, errors.New("no entropy")
		}
		return len(p), nil
	})
	f.Service = app.New(f.store, f.hasher, f.mailer, func() time.Time { return f.now }, once, "https://auth.test")
	if _, err := f.SignIn(context.Background(), "a@b.c", pw); err == nil {
		t.Fatal("a sign-in without entropy for the session id succeeded")
	}
}

func TestSignedInNamesTheSessionAndWhenThePasswordWasChecked(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "a@b.c")
	ctx := context.Background()
	first, _ := f.SignIn(ctx, "a@b.c", pw)
	second, _ := f.SignIn(ctx, "a@b.c", pw)
	f.now = t0.Add(time.Hour)

	u, sess, err := f.SignedIn(ctx, first)
	if err != nil || u.ID != id || sess.UserID != id || !sess.CreatedAt.Equal(t0) {
		t.Fatalf("SignedIn = %+v, %+v, %v", u, sess, err)
	}
	_, other, _ := f.SignedIn(ctx, second)
	if sess.ID == uuid.Nil || sess.ID == other.ID {
		t.Fatalf("session ids %v and %v are not distinct", sess.ID, other.ID)
	}
	if _, _, err := f.SignedIn(ctx, "nope"); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("an unknown token: %v", err)
	}
}

func TestSignOutEverywhereEndsEverySessionOfThatUser(t *testing.T) {
	f := newFixture()
	ada := f.signedUp(t, "a@b.c")
	f.signedUp(t, "b@b.c")
	ctx := context.Background()
	one, _ := f.SignIn(ctx, "a@b.c", pw)
	two, _ := f.SignIn(ctx, "a@b.c", pw)
	bob, _ := f.SignIn(ctx, "b@b.c", pw)
	if err := f.SignOutEverywhere(ctx, uuid.Nil, ada); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{one, two} {
		if _, err := f.Authenticate(ctx, tok); !errors.Is(err, app.ErrNoSession) {
			t.Fatalf("a session of the signed-out User survived: %v", err)
		}
	}
	if _, err := f.Authenticate(ctx, bob); err != nil {
		t.Fatalf("another User's session ended: %v", err)
	}
	boom := errors.New("boom")
	f.store.fail["DeleteOtherSessions"] = boom
	if err := f.SignOutEverywhere(ctx, uuid.Nil, ada); !errors.Is(err, boom) {
		t.Fatalf("a failing store: %v", err)
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

// gatedHasher holds every Hash until the test opens its gate, and says when one has started.
type gatedHasher struct {
	hasher
	started chan struct{}
	gate    chan struct{}
}

func (g *gatedHasher) Hash(domain.Password) (string, error) {
	g.started <- struct{}{}
	<-g.gate
	return "", app.ErrBusy
}

func TestMakingTheDecoyHoldsNoLockThatOutlastsTheHashBound(t *testing.T) {
	g := &gatedHasher{started: make(chan struct{}), gate: make(chan struct{})}
	s := app.New(newStore(), g, &mailer{}, time.Now, rand.Reader, "https://auth.test")
	done := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := s.SignIn(context.Background(), "nobody@b.c", pw)
			done <- err
		}()
	}
	for range 2 {
		select {
		case <-g.started:
		case <-time.After(2 * time.Second):
			t.Fatal("a second sign-in waits on the first one's decoy instead of the hash bound")
		}
	}
	close(g.gate)
	for range 2 {
		if err := <-done; !errors.Is(err, app.ErrBusy) {
			t.Fatalf("SignIn = %v", err)
		}
	}
}

func TestUsersListsEveryUser(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	e, _ := domain.NewEmail("ada@example.org")
	n, _ := domain.NewName("Ada")
	f.store.users[uuid.Nil] = domain.User{Email: e, Name: n}
	got, err := f.Users(ctx)
	if err != nil || len(got) != 1 || got[0].Email != e {
		t.Fatalf("Users = %+v, %v", got, err)
	}
	boom := errors.New("boom")
	f.store.fail["Users"] = boom
	if _, err := f.Users(ctx); !errors.Is(err, boom) {
		t.Fatalf("a failing store: %v", err)
	}
}

func TestUserByEmailFindsAConfirmedUser(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	e, _ := domain.NewEmail("ada@example.org")
	n, _ := domain.NewName("Ada")
	id := uuid.New()
	f.store.users[id] = domain.User{ID: id, Email: e, Name: n}
	if u, err := f.UserByEmail(ctx, " Ada@Example.org "); err != nil || u.ID != id {
		t.Fatalf("UserByEmail = %+v, %v", u, err)
	}
	if _, err := f.UserByEmail(ctx, "not an email"); !errors.Is(err, domain.ErrInvalidEmail) {
		t.Fatalf("an invalid email: %v", err)
	}
	if _, err := f.UserByEmail(ctx, "nobody@example.org"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown email: %v", err)
	}
}

func TestVouchChecksASessionByItsID(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	f.signedUp(t, "ada@example.org")
	token, _ := f.SignIn(ctx, "ada@example.org", pw)
	sess := f.store.sessions[sum(token)]
	u, got, err := f.Vouch(ctx, sess.ID)
	if err != nil || u.Email.String() != "ada@example.org" || got.ID != sess.ID {
		t.Fatalf("Vouch = %+v, %+v, %v", u, got, err)
	}
	f.now = t0.Add(time.Hour)
	if _, _, err := f.Vouch(ctx, sess.ID); err != nil || f.store.touches != 1 {
		t.Fatalf("a vouch an hour on = %v, %d touches", err, f.store.touches)
	}
	if _, _, err := f.Vouch(ctx, uuid.New()); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("an unknown session = %v", err)
	}
	_ = f.SignOut(ctx, token)
	if _, _, err := f.Vouch(ctx, sess.ID); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("a signed-out session = %v", err)
	}
	boom := errors.New("boom")
	f.store.fail["SessionByID"] = boom
	if _, _, err := f.Vouch(ctx, sess.ID); !errors.Is(err, boom) {
		t.Fatalf("a failing store = %v", err)
	}
}

func TestActiveSessionsCountsOnlyLiveSessionsAndWhoseTheyAre(t *testing.T) {
	f := newFixture()
	f.signedUp(t, "a@b.c")
	f.signedUp(t, "b@b.c")
	ctx := context.Background()
	_, _ = f.SignIn(ctx, "a@b.c", pw)
	_, _ = f.SignIn(ctx, "a@b.c", pw)
	_, _ = f.SignIn(ctx, "b@b.c", pw)
	f.now = t0.Add(31 * 24 * time.Hour)
	_, _ = f.SignIn(ctx, "b@b.c", pw)
	if sessions, users, err := f.ActiveSessions(ctx); err != nil || sessions != 1 || users != 1 {
		t.Fatalf("ActiveSessions = %d across %d, %v", sessions, users, err)
	}
}
