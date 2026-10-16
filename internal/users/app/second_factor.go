package app

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

// How a session was proved (RFC 8176).
const (
	methodPassword = "pwd"
	methodOTP      = "otp"
)

const (
	// A sign-in waits this long for its Second Factor. A User gets this many codes per window, over
	// however many sign-ins, so knowing the password does not buy unlimited guesses.
	challengeLifetime = 5 * time.Minute
	challengeAttempts = 5
	attemptWindow     = 15 * time.Minute
	// An enrolment waits this long for the first code from the new authenticator.
	enrolmentLifetime = 10 * time.Minute
)

// Challenge is a sign-in whose password was right, waiting for its Second Factor.
type Challenge struct {
	TokenHash       []byte
	UserID          uuid.UUID
	ExpiresAt       time.Time
	WindowStartedAt time.Time
}

// TOTPFactor is an authenticator app a User enrolled; its secret is kept only sealed.
type TOTPFactor struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	Label        domain.FactorLabel
	SealedSecret string
	LastStep     int64
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// TOTPEnrolment is a new authenticator waiting for its first code.
type TOTPEnrolment struct {
	UserID       uuid.UUID
	SealedSecret string
	ExpiresAt    time.Time
}

// TOTPSetup is what a User copies into their authenticator app.
type TOTPSetup struct {
	Secret string
	URI    string
}

// ProvedSecondFactor reports whether a session's methods include a Second Factor.
func ProvedSecondFactor(methods []string) bool {
	return slices.Contains(methods, methodOTP) || slices.Contains(methods, methodPasskey)
}

// Factor is a Second Factor of either kind, as a User sees it.
type Factor struct {
	ID         uuid.UUID
	Kind       string
	Label      domain.FactorLabel
	CreatedAt  time.Time
	LastUsedAt *time.Time
}

// The kinds of Second Factor.
const (
	KindTOTP    = "totp"
	KindPasskey = "passkey"
)

// SecondFactorKinds is which kinds of Second Factor a User has, authenticator apps first.
func (s *Service) SecondFactorKinds(ctx context.Context, userID uuid.UUID) ([]string, error) {
	factors, err := s.SecondFactors(ctx, userID)
	if err != nil {
		return nil, err
	}
	var kinds []string
	for _, f := range factors {
		if !slices.Contains(kinds, f.Kind) {
			kinds = append(kinds, f.Kind)
		}
	}
	slices.SortFunc(kinds, func(a, b string) int { return strings.Compare(b, a) })
	return kinds, nil
}

func (s *Service) hasSecondFactor(ctx context.Context, userID uuid.UUID) (bool, error) {
	factors, err := s.SecondFactors(ctx, userID)
	return len(factors) > 0, err
}

func (s *Service) challenge(ctx context.Context, userID uuid.UUID) (string, error) {
	token, hash, err := s.token()
	if err != nil {
		return "", err
	}
	now := s.now()
	c := Challenge{TokenHash: hash, UserID: userID, ExpiresAt: now.Add(challengeLifetime), WindowStartedAt: now}
	if err := s.store.SaveChallenge(ctx, c, now.Add(-attemptWindow)); err != nil {
		return "", err
	}
	return token, nil
}

// CompleteSignIn takes a code from one of the User's authenticators for a sign-in challenge and
// returns a session proved by both. A User gets five codes in fifteen minutes, however many times
// they sign in, and a code works once.
func (s *Service) CompleteSignIn(ctx context.Context, challenge, code string) (string, error) {
	sealer, err := s.sealerOrErr()
	if err != nil {
		return "", err
	}
	hash := hashOf(challenge)
	now := s.now()
	userID, err := s.store.SpendChallengeAttempt(ctx, hash, challengeAttempts, now)
	if errors.Is(err, ErrNotFound) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", err
	}
	factors, err := s.store.TOTPFactors(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, f := range factors {
		secret, err := sealer.Open(ctx, f.SealedSecret)
		if err != nil {
			return "", err
		}
		step, ok := domain.MatchTOTP(string(secret), code, now, f.LastStep)
		if !ok {
			continue
		}
		used, err := s.store.UseTOTPStep(ctx, f.ID, step, now)
		if err != nil {
			return "", err
		}
		if !used {
			return "", s.failed(ctx, audit.Event{User: userID, Reason: audit.ReasonSecondFactor}, ErrInvalidCode)
		}
		if err := s.store.DeleteChallenge(ctx, hash); err != nil {
			return "", err
		}
		return s.startSession(ctx, userID, []string{methodPassword, methodOTP})
	}
	return "", s.failed(ctx, audit.Event{User: userID, Reason: audit.ReasonSecondFactor}, ErrInvalidCode)
}

// SecondFactors lists a User's authenticator apps and passkeys, oldest first.
func (s *Service) SecondFactors(ctx context.Context, userID uuid.UUID) ([]Factor, error) {
	apps, err := s.store.TOTPFactors(ctx, userID)
	if err != nil {
		return nil, err
	}
	keys, err := s.store.Passkeys(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Factor, 0, len(apps)+len(keys))
	for _, f := range apps {
		out = append(out, Factor{ID: f.ID, Kind: KindTOTP, Label: f.Label, CreatedAt: f.CreatedAt, LastUsedAt: f.LastUsedAt})
	}
	for _, k := range keys {
		out = append(out, Factor{ID: k.ID, Kind: KindPasskey, Label: k.Label, CreatedAt: k.CreatedAt, LastUsedAt: k.LastUsedAt})
	}
	slices.SortStableFunc(out, func(a, b Factor) int { return a.CreatedAt.Compare(b.CreatedAt) })
	return out, nil
}

// StartTOTP makes a secret for a new authenticator, kept sealed until its first code confirms it.
func (s *Service) StartTOTP(ctx context.Context, userID uuid.UUID) (TOTPSetup, error) {
	sealer, err := s.sealerOrErr()
	if err != nil {
		return TOTPSetup{}, err
	}
	u, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return TOTPSetup{}, err
	}
	secret, err := domain.NewTOTPSecret(s.random)
	if err != nil {
		return TOTPSetup{}, err
	}
	sealed, err := sealer.Seal(ctx, []byte(secret))
	if err != nil {
		return TOTPSetup{}, err
	}
	e := TOTPEnrolment{UserID: userID, SealedSecret: sealed, ExpiresAt: s.now().Add(enrolmentLifetime)}
	if err := s.store.SaveTOTPEnrolment(ctx, e); err != nil {
		return TOTPSetup{}, err
	}
	return TOTPSetup{Secret: secret, URI: domain.TOTPURI(secret, u.Email.String())}, nil
}

// ConfirmTOTP keeps the authenticator StartTOTP set up once it gives a right code, the session it
// was confirmed in counts as proved by a Second Factor from then on, and every other session ends. So that a session someone
// took cannot buy itself that, it needs the User's password, and a User who already has an
// authenticator must be in a session that proved one.
func (s *Service) ConfirmTOTP(ctx context.Context, userID uuid.UUID, sessionToken, rawLabel, code, password string) error {
	label, err := domain.NewFactorLabel(rawLabel)
	if err != nil {
		return err
	}
	sealer, err := s.sealerOrErr()
	if err != nil {
		return err
	}
	if err := s.mayEnrol(ctx, userID, sessionToken, password); err != nil {
		return err
	}
	now := s.now()
	e, err := s.store.TOTPEnrolment(ctx, userID)
	if errors.Is(err, ErrNotFound) || err == nil && !now.Before(e.ExpiresAt) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	secret, err := sealer.Open(ctx, e.SealedSecret)
	if err != nil {
		return err
	}
	step, ok := domain.MatchTOTP(string(secret), code, now, 0)
	if !ok {
		return ErrInvalidCode
	}
	id, err := uuid.NewRandomFromReader(s.random)
	if err != nil {
		return err
	}
	f := TOTPFactor{ID: id, UserID: userID, Label: label, SealedSecret: e.SealedSecret, LastStep: step, CreatedAt: now}
	if err := s.store.CreateTOTPFactor(ctx, f); err != nil {
		return err
	}
	if err := s.store.DeleteTOTPEnrolment(ctx, userID); err != nil {
		return err
	}
	s.record(ctx, audit.Event{Action: audit.SecondFactorAdded, User: userID})
	return s.proved(ctx, userID, sessionToken, methodOTP)
}

// proved marks the session as proved by method, and ends every other session: sessions made with
// the password alone would outlive the new Second Factor, as they do not a password change.
func (s *Service) proved(ctx context.Context, userID uuid.UUID, sessionToken, method string) error {
	hash := hashOf(sessionToken)
	if err := s.store.SetSessionMethods(ctx, hash, []string{methodPassword, method}); err != nil {
		return err
	}
	_, err := s.endSessions(ctx, userID, hash)
	return err
}

// mayEnrol is nil when the password is the User's and, if they already have an authenticator, the
// session proved one.
func (s *Service) mayEnrol(ctx context.Context, userID uuid.UUID, sessionToken, password string) error {
	if err := s.checkPassword(ctx, userID, password); err != nil {
		return err
	}
	has, err := s.hasSecondFactor(ctx, userID)
	if err != nil || !has {
		return err
	}
	return s.provedSecondFactor(ctx, sessionToken)
}

// provedSecondFactor is ErrSecondFactorNeeded unless the session was proved with a Second Factor.
func (s *Service) provedSecondFactor(ctx context.Context, sessionToken string) error {
	sess, err := s.store.SessionByHash(ctx, hashOf(sessionToken))
	if err != nil {
		return err
	}
	if !ProvedSecondFactor(sess.Methods) {
		return ErrSecondFactorNeeded
	}
	return nil
}

// RemoveFactor deletes one of a User's authenticator apps or passkeys. It needs the password and a session that
// proved a Second Factor, so neither a session left open from before the User enrolled nor a
// known password alone can take the Second Factor away.
func (s *Service) RemoveFactor(ctx context.Context, userID, factorID uuid.UUID, sessionToken, password string) error {
	if err := s.checkPassword(ctx, userID, password); err != nil {
		return err
	}
	if err := s.provedSecondFactor(ctx, sessionToken); err != nil {
		return err
	}
	had, err := s.store.DeleteTOTPFactor(ctx, factorID, userID)
	if err != nil {
		return err
	}
	if !had {
		had, err = s.store.DeletePasskey(ctx, factorID, userID)
		if err != nil {
			return err
		}
	}
	if !had {
		return ErrNotFound
	}
	s.record(ctx, audit.Event{Action: audit.SecondFactorRemoved, User: userID})
	return nil
}

// checkPassword is ErrInvalidCredentials unless password is the User's.
func (s *Service) checkPassword(ctx context.Context, userID uuid.UUID, password string) error {
	u, err := s.store.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	ok, err := s.hasher.Verify(u.PasswordHash, password)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCredentials
	}
	return nil
}

func (s *Service) sealerOrErr() (Sealer, error) {
	if s.sealer == nil {
		return nil, ErrUnavailable
	}
	return s.sealer, nil
}
