package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"sync"

	"github.com/google/uuid"

	auditdomain "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var errBoom = errors.New("boom")

var testKey = sync.OnceValue(func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
})

// requests is an in-memory app.Requests; fail makes every method fail.
type requests struct {
	mu   sync.Mutex
	byID map[uuid.UUID]domain.AuthRequest
	code map[string]uuid.UUID
	fail error
}

func newRequests() *requests {
	return &requests{byID: map[uuid.UUID]domain.AuthRequest{}, code: map[string]uuid.UUID{}}
}

func (r *requests) CreateRequest(_ context.Context, req domain.AuthRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[req.ID] = req
	return r.fail
}

func (r *requests) RequestByID(_ context.Context, id uuid.UUID) (domain.AuthRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.get(id)
}

func (r *requests) get(id uuid.UUID) (domain.AuthRequest, error) {
	if r.fail != nil {
		return domain.AuthRequest{}, r.fail
	}
	req, ok := r.byID[id]
	if !ok {
		return domain.AuthRequest{}, app.ErrNotFound
	}
	return req, nil
}

func (r *requests) TakeByCodeHash(_ context.Context, h []byte) (domain.AuthRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.code[string(h)]
	req, err := r.get(id)
	delete(r.byID, id)
	delete(r.code, string(h))
	return req, err
}

func (r *requests) CompleteRequest(_ context.Context, id uuid.UUID, s domain.SignIn) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	req := r.byID[id]
	if req.SignIn != nil {
		return false, nil
	}
	req.SignIn = &s
	r.byID[id] = req
	return true, r.fail
}

func (r *requests) SaveCode(_ context.Context, id uuid.UUID, h []byte) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, other := range r.code {
		if other == id {
			return false, r.fail
		}
	}
	r.code[string(h)] = id
	return true, r.fail
}

func (r *requests) DeleteRequest(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byID, id)
	return r.fail
}

func (r *requests) AddSessionClient(context.Context, uuid.UUID, string) error { return r.fail }

func (r *requests) TakeSessionClients(context.Context, []uuid.UUID) ([]app.SessionClient, error) {
	return nil, r.fail
}

// noLogouts sends nothing: the Provider's flows never sign a session out.
type noLogouts struct{}

func (noLogouts) Send(context.Context, domain.Client, uuid.UUID, uuid.UUID) {}

type clients map[string]domain.Client

func (c clients) Client(_ context.Context, id string) (domain.Client, error) {
	if id == "broken" {
		return domain.Client{}, errBoom
	}
	cl, ok := c[id]
	if !ok {
		return domain.Client{}, app.ErrNotFound
	}
	return cl, nil
}

func (c clients) ClientForHost(context.Context, string) (domain.Client, error) {
	return domain.Client{}, app.ErrNotFound
}

// secrets knows each Secret Client's secret, as ClientSecrets would from its hash.
type secrets map[string]string

func (s secrets) Authorize(_ context.Context, clientID, secret string) error {
	if s[clientID] != secret {
		return app.ErrInvalidClient
	}
	return nil
}

var testSecrets = secrets{"web": "s3cret"}

var testClients = clients{
	"spa": {
		ID: "spa", SignIn: domain.Native, Credential: domain.PKCE, Roles: []string{"user", "viewer", "editor"},
		RedirectURIs: []string{"https://spa.test/cb"}, PostLogoutRedirectURIs: []string{"https://spa.test/bye"},
	},
	"web": {
		ID: "web", SignIn: domain.Native, Credential: domain.Secret, Roles: []string{"user", "viewer", "editor"},
		RedirectURIs: []string{"https://web.test/cb"},
	},
}

// accounts knows one session token, "tok", for Ada.
type accounts struct {
	mu         sync.Mutex
	ada        uuid.UUID
	sessionID  uuid.UUID
	signedOut  []uuid.UUID
	failClaims error
	failOut    error
	twoFactor  bool
}

func (a *accounts) HasTwoFactor(context.Context, uuid.UUID) (bool, error) { return a.twoFactor, nil }

// stepUps is every Client's step-up window in minutes; one not listed has it off.
type stepUps map[string]int

func (s stepUps) StepUp(_ context.Context, clientID string) (int, error) { return s[clientID], nil }

func (s stepUps) SetStepUp(_ context.Context, clientID string, minutes int) error {
	s[clientID] = minutes
	return nil
}

func (a *accounts) SignedIn(_ context.Context, token string) (domain.SignIn, error) {
	if token != "tok" {
		return domain.SignIn{}, app.ErrNoSession
	}
	return domain.SignIn{UserID: a.ada, SessionID: a.sessionID, At: t0}, nil
}

func (a *accounts) Claims(_ context.Context, id uuid.UUID) (app.Claims, error) {
	if id != a.ada {
		return app.Claims{}, app.ErrNotFound
	}
	return app.Claims{Email: "ada@example.org", Name: "Ada"}, a.failClaims
}

func (a *accounts) SignOutEverywhere(_ context.Context, id uuid.UUID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.signedOut = append(a.signedOut, id)
	return a.failOut
}

// access gives anyone the listed App Roles on each Client.
type access struct {
	mu    sync.Mutex
	roles map[string][]string
	fail  error
}

func newAccess() *access {
	return &access{roles: map[string][]string{"spa": {"editor", "viewer", "retired"}, "web": {"user"}}}
}

func (a *access) Roles(_ context.Context, _ uuid.UUID, clientID string) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.roles[clientID], a.fail
}

// discard records no Audit Events.
type discard struct{}

func (discard) Record(context.Context, auditdomain.Event) {}
