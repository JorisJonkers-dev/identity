package app

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var (
	// ErrConsentNeeded is a Connected App's request by a User who has not consented to it yet.
	ErrConsentNeeded = errors.New("the User has not consented to the app")
	// ErrNotConnectable is a Connected App's request for a Client that takes none, or asks for
	// step-up (identity#23).
	ErrNotConnectable = errors.New("the Client takes no Connected Apps")
)

// Consents is the connected context as sign-in needs it (identity#23).
type Consents interface {
	// Consented is nil when the User consented to the registration acting for them on the Client,
	// ErrConsentNeeded when they have not, and ErrNotConnectable or ErrNoAccess when they cannot.
	Consented(ctx context.Context, userID uuid.UUID, registrationID, clientID string) error
	// Consent records the User's consent, failing as Consented does.
	Consent(ctx context.Context, userID uuid.UUID, registrationID, clientID string) error
	// Name is what a registration calls itself.
	Name(ctx context.Context, registrationID string) (string, error)
}

// SetConsents lets registered tools act for Users; until it is called, none can. It is set once,
// before the Provider serves.
func (s *Service) SetConsents(c Consents) { s.consents = c }

// Prompt is what the consent page asks a User about.
type Prompt struct {
	// App is what the tool called itself; nothing checks it.
	App         string
	ClientLabel string
	Roles       []string
	RedirectURI string
}

// Prompt is what a Connected App's request asks the User signed in by sessionToken to consent to.
func (s *Service) Prompt(ctx context.Context, id uuid.UUID, sessionToken string) (Prompt, error) {
	r, signIn, err := s.connected(ctx, id, sessionToken)
	if err != nil {
		return Prompt{}, err
	}
	c, err := s.clients.Client(ctx, r.Resource)
	if err != nil {
		return Prompt{}, err
	}
	roles, err := s.admit(ctx, signIn, r.Resource)
	if err != nil {
		return Prompt{}, err
	}
	name, err := s.consents.Name(ctx, r.ClientID)
	if err != nil {
		return Prompt{}, err
	}
	return Prompt{App: name, ClientLabel: c.Label, Roles: roles, RedirectURI: r.RedirectURI}, nil
}

// Approve records the User's consent and completes the request, so the tool gets its code.
func (s *Service) Approve(ctx context.Context, id uuid.UUID, sessionToken string) error {
	r, signIn, err := s.connected(ctx, id, sessionToken)
	if err != nil {
		return err
	}
	if _, err := s.admit(ctx, signIn, r.Resource); err != nil {
		return err
	}
	if err := s.consents.Consent(ctx, signIn.UserID, r.ClientID, r.Resource); err != nil {
		return err
	}
	return s.complete(ctx, r, signIn)
}

// Deny forgets the request, and answers where to tell the tool so: its redirect URI and state.
func (s *Service) Deny(ctx context.Context, id uuid.UUID, sessionToken string) (domain.AuthRequest, error) {
	r, _, err := s.connected(ctx, id, sessionToken)
	if err != nil {
		return domain.AuthRequest{}, err
	}
	return r, s.requests.DeleteRequest(ctx, id)
}

// connected is a live Connected App's request and the User signed in to answer it.
func (s *Service) connected(ctx context.Context, id uuid.UUID, sessionToken string) (domain.AuthRequest, domain.SignIn, error) {
	r, err := s.Request(ctx, id)
	if err != nil {
		return domain.AuthRequest{}, domain.SignIn{}, err
	}
	if r.Resource == "" || s.consents == nil {
		return domain.AuthRequest{}, domain.SignIn{}, ErrNotFound
	}
	signIn, err := s.accounts.SignedIn(ctx, sessionToken)
	if err != nil {
		return domain.AuthRequest{}, domain.SignIn{}, err
	}
	return r, signIn, nil
}
