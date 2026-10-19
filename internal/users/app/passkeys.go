package app

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// methodPasskey is RFC 8176's proof with a hardware-held key, as a passkey is.
const methodPasskey = "hwk"

var (
	ErrInvalidPasskey = errors.New("the passkey's answer is not accepted")
	ErrNoPasskeys     = errors.New("no passkeys")
)

// WebAuthn runs passkey ceremonies. Options go to the browser as JSON; a ceremony is kept until
// the browser answers.
type WebAuthn interface {
	BeginRegistration(u PasskeyUser) (options, ceremony []byte, err error)
	FinishRegistration(u PasskeyUser, ceremony, response []byte) (NewPasskey, error)
	BeginLogin(u PasskeyUser) (options, ceremony []byte, err error)
	// FinishLogin returns the passkey the assertion was made with, its counter moved on.
	FinishLogin(u PasskeyUser, ceremony, response []byte) (NewPasskey, error)
}

// PasskeyUser is a User as a ceremony needs them, with their passkeys as WebAuthn recorded them.
type PasskeyUser struct {
	ID          uuid.UUID
	Email, Name string
	Credentials [][]byte
}

// NewPasskey is what a ceremony produced: the credential's id and WebAuthn's record of it.
type NewPasskey struct {
	CredentialID []byte
	Credential   []byte
}

// Passkey is one of a User's passkeys.
type Passkey struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Label        domain.FactorLabel
	CredentialID []byte
	Credential   []byte
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// PasskeyEnrolment is a registration waiting for the browser.
type PasskeyEnrolment struct {
	UserID    uuid.UUID
	Ceremony  []byte
	ExpiresAt time.Time
}

// WithWebAuthn lets Users add passkeys.
func WithWebAuthn(w WebAuthn) Option { return func(s *Service) { s.webauthn = w } }

func (s *Service) passkeyUser(ctx context.Context, userID uuid.UUID) (PasskeyUser, error) {
	pu, _, err := s.passkeyUserAndKeys(ctx, userID)
	return pu, err
}

func (s *Service) passkeyUserAndKeys(ctx context.Context, userID uuid.UUID) (PasskeyUser, []Passkey, error) {
	u, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return PasskeyUser{}, nil, err
	}
	keys, err := s.store.Passkeys(ctx, userID)
	if err != nil {
		return PasskeyUser{}, nil, err
	}
	creds := make([][]byte, 0, len(keys))
	for _, k := range keys {
		creds = append(creds, k.Credential)
	}
	return PasskeyUser{ID: userID, Email: u.Email.String(), Name: u.Name.String(), Credentials: creds}, keys, nil
}

func (s *Service) webauthnOrErr() (WebAuthn, error) {
	if s.webauthn == nil {
		return nil, ErrUnavailable
	}
	return s.webauthn, nil
}

// StartPasskey returns the options for the browser to make a passkey, kept for ten minutes.
func (s *Service) StartPasskey(ctx context.Context, userID uuid.UUID) ([]byte, error) {
	wa, err := s.webauthnOrErr()
	if err != nil {
		return nil, err
	}
	pu, err := s.passkeyUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	options, ceremony, err := wa.BeginRegistration(pu)
	if err != nil {
		return nil, err
	}
	e := PasskeyEnrolment{UserID: userID, Ceremony: ceremony, ExpiresAt: s.now().Add(enrolmentLifetime)}
	if err := s.store.SavePasskeyEnrolment(ctx, e); err != nil {
		return nil, err
	}
	return options, nil
}

// ConfirmPasskey keeps the passkey the browser made for StartPasskey's options. It follows the
// rules ConfirmTOTP does (docs/adr/0015): the password, a proved session for a User who already
// has a Second Factor, and every other session ended.
func (s *Service) ConfirmPasskey(ctx context.Context, userID uuid.UUID, sessionToken, rawLabel string, response []byte, password string) error {
	label, err := domain.NewFactorLabel(rawLabel)
	if err != nil {
		return err
	}
	wa, err := s.webauthnOrErr()
	if err != nil {
		return err
	}
	if err := s.mayEnrol(ctx, userID, sessionToken, password); err != nil {
		return err
	}
	now := s.now()
	e, err := s.store.PasskeyEnrolment(ctx, userID)
	if errors.Is(err, ErrNotFound) || err == nil && !now.Before(e.ExpiresAt) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	pu, err := s.passkeyUser(ctx, userID)
	if err != nil {
		return err
	}
	made, err := wa.FinishRegistration(pu, e.Ceremony, response)
	if err != nil {
		return ErrInvalidPasskey
	}
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return err
	}
	p := Passkey{ID: id, UserID: userID, Label: label, CredentialID: made.CredentialID, Credential: made.Credential, CreatedAt: now}
	if err := s.store.CreatePasskey(ctx, p); err != nil {
		return err
	}
	if err := s.store.DeletePasskeyEnrolment(ctx, userID); err != nil {
		return err
	}
	s.record(ctx, audit.Event{Action: audit.SecondFactorAdded, User: userID})
	return s.proved(ctx, userID, sessionToken, methodPasskey)
}

// PasskeyOptions starts the passkey step of a sign-in the challenge names.
func (s *Service) PasskeyOptions(ctx context.Context, challenge string) ([]byte, error) {
	return s.passkeyOptions(ctx, hashOf(challenge))
}

func (s *Service) passkeyOptions(ctx context.Context, hash []byte) ([]byte, error) {
	wa, err := s.webauthnOrErr()
	if err != nil {
		return nil, err
	}
	now := s.now()
	userID, _, err := s.store.ChallengeCeremony(ctx, hash, now)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	pu, err := s.passkeyUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(pu.Credentials) == 0 {
		return nil, ErrNoPasskeys
	}
	options, ceremony, err := wa.BeginLogin(pu)
	if err != nil {
		return nil, err
	}
	err = s.store.SetChallengeCeremony(ctx, hash, ceremony, now)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrInvalidToken
	}
	if err != nil {
		return nil, err
	}
	return options, nil
}

// CompleteSignInWithPasskey takes the browser's passkey assertion for the sign-in and returns a
// session proved by password and passkey. A refused assertion counts as a wrong code does.
func (s *Service) CompleteSignInWithPasskey(ctx context.Context, challenge string, response []byte) (string, error) {
	userID, err := s.answerWithPasskey(ctx, hashOf(challenge), response)
	if err != nil {
		return "", err
	}
	return s.startSession(ctx, userID, []string{methodPassword, methodPasskey})
}

// answerWithPasskey spends one attempt of the challenge on the browser's passkey assertion, and on
// an accepted one removes the challenge and returns the User.
func (s *Service) answerWithPasskey(ctx context.Context, hash, response []byte) (uuid.UUID, error) {
	wa, err := s.webauthnOrErr()
	if err != nil {
		return uuid.Nil, err
	}
	now := s.now()
	userID, err := s.store.SpendChallengeAttempt(ctx, hash, challengeAttempts, now)
	if errors.Is(err, ErrNotFound) {
		return uuid.Nil, ErrInvalidToken
	}
	if err != nil {
		return uuid.Nil, err
	}
	refused := audit.Event{User: userID, Reason: audit.ReasonSecondFactor}
	// The ceremony is taken, not read: of two requests with one assertion, one gets it.
	_, ceremony, err := s.store.TakeChallengeCeremony(ctx, hash, now)
	if errors.Is(err, ErrNotFound) {
		return uuid.Nil, s.failed(ctx, refused, ErrInvalidPasskey)
	}
	if err != nil {
		return uuid.Nil, err
	}
	pu, keys, err := s.passkeyUserAndKeys(ctx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	used, err := wa.FinishLogin(pu, ceremony, response)
	if err != nil {
		return uuid.Nil, s.failed(ctx, refused, ErrInvalidPasskey)
	}
	// The counter moves on only from the record it was checked against, so two sign-ins with a
	// copied key cannot both pass on one stored counter.
	i := slices.IndexFunc(keys, func(k Passkey) bool { return bytes.Equal(k.CredentialID, used.CredentialID) })
	if i < 0 {
		return uuid.Nil, s.failed(ctx, refused, ErrInvalidPasskey)
	}
	kept, err := s.store.UsePasskey(ctx, userID, used, keys[i].Credential, now)
	if err != nil {
		return uuid.Nil, err
	}
	if !kept {
		return uuid.Nil, s.failed(ctx, refused, ErrInvalidPasskey)
	}
	return userID, s.store.DeleteChallenge(ctx, hash)
}

// ChallengeKinds is which kinds of Second Factor the User of a sign-in challenge has.
func (s *Service) ChallengeKinds(ctx context.Context, challenge string) ([]string, error) {
	userID, _, err := s.store.ChallengeCeremony(ctx, hashOf(challenge), s.now())
	if err != nil {
		return nil, err
	}
	return s.SecondFactorKinds(ctx, userID)
}
