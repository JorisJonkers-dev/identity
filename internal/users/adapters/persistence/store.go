// Package persistence is the users context's Postgres adapter: app.Store over the sqlc queries.
package persistence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
	"github.com/JorisJonkers-dev/identity/internal/users/domain"
)

const uniqueViolation = "23505"

// Store is the app.Store.
type Store struct{ q queries.Querier }

// New returns a Store over q.
func New(q queries.Querier) *Store { return &Store{q: q} }

var _ app.Store = (*Store)(nil)

func (s *Store) CreateUser(ctx context.Context, u domain.User) error {
	err := s.q.CreateUser(ctx, queries.CreateUserParams{
		ID: u.ID, Email: u.Email.String(), Name: u.Name.String(), PasswordHash: u.PasswordHash, CreatedAt: u.CreatedAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return app.ErrEmailTaken
	}
	return err
}

func (s *Store) UserByEmail(ctx context.Context, e domain.Email) (domain.User, error) {
	row, err := s.q.UserByEmail(ctx, e.String())
	if err != nil {
		return domain.User{}, notFound(err)
	}
	return toUser(row.ID, row.Email, row.Name, row.PasswordHash, row.CreatedAt)
}

func (s *Store) UserByID(ctx context.Context, id uuid.UUID) (domain.User, error) {
	row, err := s.q.UserByID(ctx, id)
	if err != nil {
		return domain.User{}, notFound(err)
	}
	return toUser(row.ID, row.Email, row.Name, row.PasswordHash, row.CreatedAt)
}

func (s *Store) CountActiveSessions(ctx context.Context, now time.Time) (sessions, users int, err error) {
	r, err := s.q.CountActiveSessions(ctx, now)
	return int(r.Sessions), int(r.Users), err
}

func (s *Store) Users(ctx context.Context) ([]domain.User, error) {
	rows, err := s.q.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]domain.User, 0, len(rows))
	for _, r := range rows {
		u, err := toUser(r.ID, r.Email, r.Name, r.PasswordHash, r.CreatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *Store) UpdateName(ctx context.Context, id uuid.UUID, n domain.Name) error {
	return s.q.UpdateUserName(ctx, queries.UpdateUserNameParams{ID: id, Name: n.String()})
}

func (s *Store) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	return s.q.UpdateUserPassword(ctx, queries.UpdateUserPasswordParams{ID: id, PasswordHash: hash})
}

func (s *Store) CreateSignup(ctx context.Context, su app.Signup) error {
	return s.q.CreateSignup(ctx, queries.CreateSignupParams{
		TokenHash: su.TokenHash, Email: su.Email.String(), Name: su.Name.String(), PasswordHash: su.PasswordHash, ExpiresAt: su.ExpiresAt,
	})
}

func (s *Store) SignupByHash(ctx context.Context, hash []byte) (app.Signup, error) {
	row, err := s.q.SignupByHash(ctx, hash)
	if err != nil {
		return app.Signup{}, notFound(err)
	}
	email, err := domain.NewEmail(row.Email)
	if err != nil {
		return app.Signup{}, err
	}
	name, err := domain.NewName(row.Name)
	if err != nil {
		return app.Signup{}, err
	}
	return app.Signup{TokenHash: hash, Email: email, Name: name, PasswordHash: row.PasswordHash, ExpiresAt: row.ExpiresAt}, nil
}

func (s *Store) DeleteSignup(ctx context.Context, hash []byte) error {
	return s.q.DeleteSignup(ctx, hash)
}

func (s *Store) CreateSession(ctx context.Context, sess app.Session) error {
	return s.q.CreateSession(ctx, queries.CreateSessionParams{
		TokenHash: sess.TokenHash, ID: sess.ID, UserID: sess.UserID, CreatedAt: sess.CreatedAt, ExpiresAt: sess.ExpiresAt,
		Methods: sess.Methods, TwoFactorAt: sess.TwoFactorAt, Device: sess.Device,
	})
}

func (s *Store) SessionByHash(ctx context.Context, hash []byte) (app.Session, error) {
	row, err := s.q.SessionByHash(ctx, hash)
	if err != nil {
		return app.Session{}, notFound(err)
	}
	return app.Session{
		TokenHash: row.TokenHash, ID: row.ID, UserID: row.UserID,
		CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt, Methods: row.Methods,
		TwoFactorAt: row.TwoFactorAt,
	}, nil
}

func (s *Store) SessionByID(ctx context.Context, id uuid.UUID) (app.Session, error) {
	row, err := s.q.SessionByID(ctx, id)
	if err != nil {
		return app.Session{}, notFound(err)
	}
	return app.Session{
		TokenHash: row.TokenHash, ID: row.ID, UserID: row.UserID,
		CreatedAt: row.CreatedAt, LastSeenAt: row.LastSeenAt, ExpiresAt: row.ExpiresAt, Methods: row.Methods,
		TwoFactorAt: row.TwoFactorAt,
	}, nil
}

func (s *Store) SessionsOf(ctx context.Context, userID uuid.UUID, now time.Time) ([]app.Session, error) {
	rows, err := s.q.SessionsOfUser(ctx, queries.SessionsOfUserParams{UserID: userID, ExpiresAt: now})
	if err != nil {
		return nil, err
	}
	out := make([]app.Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, app.Session{TokenHash: r.TokenHash, ID: r.ID, UserID: userID, Device: r.Device, CreatedAt: r.CreatedAt, LastSeenAt: r.LastSeenAt})
	}
	return out, nil
}

func (s *Store) DeleteSession(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	n, err := s.q.DeleteSessionOfUser(ctx, queries.DeleteSessionOfUserParams{ID: id, UserID: userID})
	return n > 0, err
}

func (s *Store) SaveEmailChange(ctx context.Context, c app.EmailChange) error {
	return s.q.SaveEmailChange(ctx, queries.SaveEmailChangeParams{UserID: c.UserID, Email: c.Email.String(), TokenHash: c.TokenHash, ExpiresAt: c.ExpiresAt})
}

func (s *Store) EmailChangeOf(ctx context.Context, userID uuid.UUID) (app.EmailChange, error) {
	r, err := s.q.EmailChangeOfUser(ctx, userID)
	if err != nil {
		return app.EmailChange{}, notFound(err)
	}
	return emailChange(r.UserID, r.Email, r.TokenHash, r.ExpiresAt)
}

func (s *Store) TakeEmailChange(ctx context.Context, hash []byte) (app.EmailChange, error) {
	r, err := s.q.TakeEmailChange(ctx, hash)
	if err != nil {
		return app.EmailChange{}, notFound(err)
	}
	return emailChange(r.UserID, r.Email, r.TokenHash, r.ExpiresAt)
}

func emailChange(userID uuid.UUID, rawEmail string, hash []byte, expires time.Time) (app.EmailChange, error) {
	email, err := domain.NewEmail(rawEmail)
	return app.EmailChange{UserID: userID, Email: email, TokenHash: hash, ExpiresAt: expires}, err
}

func (s *Store) DeleteEmailChange(ctx context.Context, userID uuid.UUID) error {
	return s.q.DeleteEmailChange(ctx, userID)
}

func (s *Store) UpdateEmail(ctx context.Context, userID uuid.UUID, email domain.Email) error {
	err := s.q.UpdateEmail(ctx, queries.UpdateEmailParams{ID: userID, Email: email.String()})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return app.ErrEmailTaken
	}
	return err
}

func (s *Store) TouchSession(ctx context.Context, hash []byte, lastSeen, expires time.Time) error {
	return s.q.TouchSession(ctx, queries.TouchSessionParams{TokenHash: hash, LastSeenAt: lastSeen, ExpiresAt: expires})
}

func (s *Store) DeleteOtherSessions(ctx context.Context, userID uuid.UUID, keep []byte) ([]uuid.UUID, error) {
	return s.q.DeleteOtherSessions(ctx, queries.DeleteOtherSessionsParams{UserID: userID, Keep: keep})
}

func (s *Store) SetSessionMethods(ctx context.Context, hash []byte, methods []string, at time.Time) error {
	return s.q.SetSessionMethods(ctx, queries.SetSessionMethodsParams{TokenHash: hash, Methods: methods, TwoFactorAt: &at})
}

func (s *Store) SaveChallenge(ctx context.Context, c app.Challenge, windowSince time.Time) error {
	return s.q.UpsertChallenge(ctx, queries.UpsertChallengeParams{
		TokenHash: c.TokenHash, UserID: c.UserID, ExpiresAt: c.ExpiresAt, WindowStartedAt: c.WindowStartedAt, WindowSince: windowSince,
		SessionHash: c.SessionHash,
	})
}

func (s *Store) StepUpChallenge(ctx context.Context, sessionHash []byte) ([]byte, error) {
	hash, err := s.q.StepUpChallenge(ctx, sessionHash)
	return hash, notFound(err)
}

func (s *Store) SpendChallengeAttempt(ctx context.Context, hash []byte, maxAttempts int, now time.Time) (uuid.UUID, error) {
	id, err := s.q.SpendChallengeAttempt(ctx, queries.SpendChallengeAttemptParams{TokenHash: hash, Attempts: int32(maxAttempts), ExpiresAt: now}) //nolint:gosec // a small constant
	return id, notFound(err)
}

func (s *Store) DeleteChallenge(ctx context.Context, hash []byte) error {
	return s.q.DeleteChallenge(ctx, hash)
}

func (s *Store) TOTPFactors(ctx context.Context, userID uuid.UUID) ([]app.TOTPFactor, error) {
	rows, err := s.q.TOTPFactorsOfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	factors := make([]app.TOTPFactor, 0, len(rows))
	for _, r := range rows {
		label, err := domain.NewFactorLabel(r.Label)
		if err != nil {
			return nil, err
		}
		factors = append(factors, app.TOTPFactor{
			ID: r.ID, UserID: r.UserID, Label: label, SealedSecret: r.SealedSecret,
			LastStep: r.LastStep, CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt,
		})
	}
	return factors, nil
}

func (s *Store) CreateTOTPFactor(ctx context.Context, f app.TOTPFactor) error {
	return s.q.CreateTOTPFactor(ctx, queries.CreateTOTPFactorParams{
		ID: f.ID, UserID: f.UserID, Label: f.Label.String(), SealedSecret: f.SealedSecret, LastStep: f.LastStep, CreatedAt: f.CreatedAt,
	})
}

func (s *Store) UseTOTPStep(ctx context.Context, id uuid.UUID, step int64, at time.Time) (bool, error) {
	n, err := s.q.UseTOTPStep(ctx, queries.UseTOTPStepParams{ID: id, LastStep: step, LastUsedAt: &at})
	return n == 1, err
}

func (s *Store) DeleteTOTPFactor(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	n, err := s.q.DeleteTOTPFactor(ctx, queries.DeleteTOTPFactorParams{ID: id, UserID: userID})
	return n == 1, err
}

func (s *Store) SaveTOTPEnrolment(ctx context.Context, e app.TOTPEnrolment) error {
	return s.q.UpsertTOTPEnrolment(ctx, queries.UpsertTOTPEnrolmentParams{UserID: e.UserID, SealedSecret: e.SealedSecret, ExpiresAt: e.ExpiresAt})
}

func (s *Store) TOTPEnrolment(ctx context.Context, userID uuid.UUID) (app.TOTPEnrolment, error) {
	r, err := s.q.TOTPEnrolment(ctx, userID)
	if err != nil {
		return app.TOTPEnrolment{}, notFound(err)
	}
	return app.TOTPEnrolment{UserID: userID, SealedSecret: r.SealedSecret, ExpiresAt: r.ExpiresAt}, nil
}

func (s *Store) DeleteTOTPEnrolment(ctx context.Context, userID uuid.UUID) error {
	return s.q.DeleteTOTPEnrolment(ctx, userID)
}

func (s *Store) Passkeys(ctx context.Context, userID uuid.UUID) ([]app.Passkey, error) {
	rows, err := s.q.PasskeysOfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	keys := make([]app.Passkey, 0, len(rows))
	for _, r := range rows {
		label, err := domain.NewFactorLabel(r.Label)
		if err != nil {
			return nil, err
		}
		keys = append(keys, app.Passkey{
			ID: r.ID, UserID: r.UserID, Label: label, CredentialID: r.CredentialID, Credential: r.Credential,
			CreatedAt: r.CreatedAt, LastUsedAt: r.LastUsedAt,
		})
	}
	return keys, nil
}

func (s *Store) CreatePasskey(ctx context.Context, p app.Passkey) error {
	err := s.q.CreatePasskey(ctx, queries.CreatePasskeyParams{
		ID: p.ID, UserID: p.UserID, Label: p.Label.String(), CredentialID: p.CredentialID, Credential: p.Credential, CreatedAt: p.CreatedAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return app.ErrInvalidPasskey
	}
	return err
}

func (s *Store) UsePasskey(ctx context.Context, userID uuid.UUID, used app.NewPasskey, previous []byte, at time.Time) (bool, error) {
	n, err := s.q.UsePasskey(ctx, queries.UsePasskeyParams{
		UserID: userID, CredentialID: used.CredentialID, Credential: used.Credential, LastUsedAt: &at, Previous: previous,
	})
	return n == 1, err
}

func (s *Store) TakeChallengeCeremony(ctx context.Context, hash []byte, now time.Time) (uuid.UUID, []byte, error) {
	r, err := s.q.TakeChallengeCeremony(ctx, queries.TakeChallengeCeremonyParams{TokenHash: hash, Now: now})
	if err != nil {
		return uuid.Nil, nil, notFound(err)
	}
	return r.UserID, r.PasskeyCeremony, nil
}

func (s *Store) DeletePasskey(ctx context.Context, id, userID uuid.UUID) (bool, error) {
	n, err := s.q.DeletePasskey(ctx, queries.DeletePasskeyParams{ID: id, UserID: userID})
	return n == 1, err
}

func (s *Store) SavePasskeyEnrolment(ctx context.Context, e app.PasskeyEnrolment) error {
	return s.q.UpsertPasskeyEnrolment(ctx, queries.UpsertPasskeyEnrolmentParams{UserID: e.UserID, Ceremony: e.Ceremony, ExpiresAt: e.ExpiresAt})
}

func (s *Store) PasskeyEnrolment(ctx context.Context, userID uuid.UUID) (app.PasskeyEnrolment, error) {
	r, err := s.q.PasskeyEnrolment(ctx, userID)
	if err != nil {
		return app.PasskeyEnrolment{}, notFound(err)
	}
	return app.PasskeyEnrolment{UserID: userID, Ceremony: r.Ceremony, ExpiresAt: r.ExpiresAt}, nil
}

func (s *Store) DeletePasskeyEnrolment(ctx context.Context, userID uuid.UUID) error {
	return s.q.DeletePasskeyEnrolment(ctx, userID)
}

func (s *Store) ChallengeCeremony(ctx context.Context, hash []byte, now time.Time) (uuid.UUID, []byte, error) {
	r, err := s.q.ChallengeCeremony(ctx, queries.ChallengeCeremonyParams{TokenHash: hash, ExpiresAt: now})
	if err != nil {
		return uuid.Nil, nil, notFound(err)
	}
	return r.UserID, r.PasskeyCeremony, nil
}

func (s *Store) SetChallengeCeremony(ctx context.Context, hash, ceremony []byte, now time.Time) error {
	n, err := s.q.SetChallengeCeremony(ctx, queries.SetChallengeCeremonyParams{TokenHash: hash, PasskeyCeremony: ceremony, ExpiresAt: now})
	if err == nil && n == 0 {
		return app.ErrNotFound
	}
	return err
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrNotFound
	}
	return err
}

// toUser rebuilds a User from a row, through the domain's constructors, so a row the rules would
// refuse is an error rather than a User nobody could have made.
func toUser(id uuid.UUID, rawEmail, rawName, hash string, created time.Time) (domain.User, error) {
	email, err := domain.NewEmail(rawEmail)
	if err != nil {
		return domain.User{}, err
	}
	name, err := domain.NewName(rawName)
	if err != nil {
		return domain.User{}, err
	}
	return domain.User{ID: id, Email: email, Name: name, PasswordHash: hash, CreatedAt: created}, nil
}
