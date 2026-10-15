package app_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/signin/app"
	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

var t0 = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

var errBoom = errors.New("boom")

// requests is an in-memory app.Requests; fail makes the named method return its error.
type requests struct {
	byID map[uuid.UUID]domain.AuthRequest
	code map[string]uuid.UUID
	fail map[string]error
}

func (r *requests) CreateRequest(_ context.Context, req domain.AuthRequest) error {
	if err := r.fail["CreateRequest"]; err != nil {
		return err
	}
	r.byID[req.ID] = req
	return nil
}

func (r *requests) RequestByID(_ context.Context, id uuid.UUID) (domain.AuthRequest, error) {
	if err := r.fail["RequestByID"]; err != nil {
		return domain.AuthRequest{}, err
	}
	req, ok := r.byID[id]
	if !ok {
		return domain.AuthRequest{}, app.ErrNotFound
	}
	return req, nil
}

func (r *requests) TakeByCodeHash(ctx context.Context, h []byte) (domain.AuthRequest, error) {
	id, ok := r.code[string(h)]
	if !ok {
		return domain.AuthRequest{}, app.ErrNotFound
	}
	req, err := r.RequestByID(ctx, id)
	delete(r.byID, id)
	delete(r.code, string(h))
	return req, err
}

func (r *requests) CompleteRequest(_ context.Context, id uuid.UUID, s domain.SignIn) (bool, error) {
	if err := r.fail["CompleteRequest"]; err != nil {
		return false, err
	}
	req, ok := r.byID[id]
	if !ok || req.SignIn != nil {
		return false, nil
	}
	req.SignIn = &s
	r.byID[id] = req
	return true, nil
}

func (r *requests) SaveCode(_ context.Context, id uuid.UUID, h []byte) (bool, error) {
	if err := r.fail["SaveCode"]; err != nil {
		return false, err
	}
	for _, other := range r.code {
		if other == id {
			return false, nil
		}
	}
	r.code[string(h)] = id
	return true, nil
}

func (r *requests) DeleteRequest(_ context.Context, id uuid.UUID) error {
	delete(r.byID, id)
	return nil
}

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

// accounts knows one session token, "tok", for Ada.
type accounts struct {
	ada        uuid.UUID
	sessionID  uuid.UUID
	signedOut  uuid.UUID
	failClaims error
}

func (a *accounts) SignedIn(_ context.Context, token string) (domain.SignIn, error) {
	if token != "tok" {
		return domain.SignIn{}, app.ErrNoSession
	}
	return domain.SignIn{UserID: a.ada, SessionID: a.sessionID, At: t0}, nil
}

func (a *accounts) Claims(context.Context, uuid.UUID) (app.Claims, error) {
	return app.Claims{Email: "ada@example.org", Name: "Ada"}, a.failClaims
}

func (a *accounts) SignOutEverywhere(_ context.Context, id uuid.UUID) error {
	a.signedOut = id
	return nil
}

type fixture struct {
	*app.Service
	requests *requests
	accounts *accounts
	now      time.Time
}

type counter struct{ n byte }

func (c *counter) Read(p []byte) (int, error) {
	c.n++
	for i := range p {
		p[i] = c.n
	}
	return len(p), nil
}

type broken struct{}

func (broken) Read([]byte) (int, error) { return 0, errBoom }

func secretHash(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}

func newFixture() *fixture {
	f := &fixture{
		requests: &requests{byID: map[uuid.UUID]domain.AuthRequest{}, code: map[string]uuid.UUID{}, fail: map[string]error{}},
		accounts: &accounts{ada: uuid.New(), sessionID: uuid.New()},
		now:      t0,
	}
	cs := clients{
		"spa": {ID: "spa", RedirectURIs: []string{"https://spa.test/cb"}},
		"web": {ID: "web", RedirectURIs: []string{"https://web.test/cb"}, SecretHash: secretHash("s3cret")},
	}
	f.Service = app.New(f.requests, cs, f.accounts, func() time.Time { return f.now }, &counter{})
	return f
}

func (f *fixture) started(t *testing.T) domain.AuthRequest {
	t.Helper()
	r, err := f.Start(context.Background(), domain.AuthRequest{ClientID: "spa", State: "st"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestStartKeepsTheRequestForTenMinutes(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	r := f.started(t)
	if r.ID == uuid.Nil || !r.CreatedAt.Equal(t0) || !r.ExpiresAt.Equal(t0.Add(10*time.Minute)) {
		t.Fatalf("Start = %+v", r)
	}
	if got, err := f.Request(ctx, r.ID); err != nil || got.State != "st" {
		t.Fatalf("Request = %+v, %v", got, err)
	}
	f.now = r.ExpiresAt
	if _, err := f.Request(ctx, r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an expired request: %v", err)
	}
	if _, err := f.Request(ctx, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown request: %v", err)
	}
}

func TestStartFailures(t *testing.T) {
	f := newFixture()
	f.Service = app.New(f.requests, clients{}, f.accounts, time.Now, broken{})
	if _, err := f.Start(context.Background(), domain.AuthRequest{}); !errors.Is(err, errBoom) {
		t.Fatalf("no entropy: %v", err)
	}
	f = newFixture()
	f.requests.fail["CreateRequest"] = errBoom
	if _, err := f.Start(context.Background(), domain.AuthRequest{}); !errors.Is(err, errBoom) {
		t.Fatalf("a failing store: %v", err)
	}
}

func TestASessionCompletesARequestOnce(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	r := f.started(t)
	if err := f.Complete(ctx, r.ID, "nope"); !errors.Is(err, app.ErrNoSession) {
		t.Fatalf("no session: %v", err)
	}
	if err := f.Complete(ctx, r.ID, "tok"); err != nil {
		t.Fatal(err)
	}
	got, _ := f.Request(ctx, r.ID)
	if got.SignIn == nil || got.SignIn.UserID != f.accounts.ada || got.SignIn.SessionID != f.accounts.sessionID || !got.SignIn.At.Equal(t0) {
		t.Fatalf("completed request = %+v", got.SignIn)
	}
	if err := f.Complete(ctx, r.ID, "tok"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("completing twice: %v", err)
	}
	if err := f.Complete(ctx, uuid.New(), "tok"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("an unknown request: %v", err)
	}
	r = f.started(t)
	f.requests.fail["CompleteRequest"] = errBoom
	if err := f.Complete(ctx, r.ID, "tok"); !errors.Is(err, errBoom) {
		t.Fatalf("a failing store: %v", err)
	}
}

func TestACodeIsStoredHashedAndIssuedOnce(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	r := f.started(t)
	_ = f.Complete(ctx, r.ID, "tok")
	if err := f.SaveCode(ctx, r.ID, "the-code"); err != nil {
		t.Fatal(err)
	}
	for h := range f.requests.code {
		if bytes.Contains([]byte(h), []byte("the-code")) {
			t.Fatal("the code is stored as it is")
		}
	}
	if got, err := f.Redeem(ctx, "the-code"); err != nil || got.ID != r.ID {
		t.Fatalf("Redeem = %+v, %v", got, err)
	}
	if _, err := f.Redeem(ctx, "the-code"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a code redeemed twice: %v", err)
	}
	if _, err := f.Request(ctx, r.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a redeemed request is still there: %v", err)
	}
	if _, err := f.Redeem(ctx, "wrong"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a wrong code: %v", err)
	}
	r = f.started(t)
	_ = f.Complete(ctx, r.ID, "tok")
	_ = f.SaveCode(ctx, r.ID, "first")
	if err := f.SaveCode(ctx, r.ID, "another"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a second code: %v", err)
	}
	f.requests.fail["SaveCode"] = errBoom
	if err := f.SaveCode(ctx, r.ID, "x"); !errors.Is(err, errBoom) {
		t.Fatalf("a failing store: %v", err)
	}
	if err := f.Finish(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Redeem(ctx, "first"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("a finished request: %v", err)
	}
}

func TestAuthorizeSecret(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	if err := f.AuthorizeSecret(ctx, "web", "s3cret"); err != nil {
		t.Fatalf("the right secret: %v", err)
	}
	for _, c := range [][2]string{{"web", "wrong"}, {"spa", ""}, {"nobody", "s3cret"}} {
		if err := f.AuthorizeSecret(ctx, c[0], c[1]); !errors.Is(err, app.ErrInvalidClient) {
			t.Errorf("AuthorizeSecret(%q, %q) = %v", c[0], c[1], err)
		}
	}
	if err := f.AuthorizeSecret(ctx, "broken", "x"); !errors.Is(err, errBoom) {
		t.Fatalf("a failing client source: %v", err)
	}
	if c, err := f.Client(ctx, "spa"); err != nil || !c.Public() {
		t.Fatalf("Client = %+v, %v", c, err)
	}
}

func TestAccountsPassThrough(t *testing.T) {
	f := newFixture()
	ctx := context.Background()
	if c, err := f.Claims(ctx, f.accounts.ada); err != nil || c.Email != "ada@example.org" {
		t.Fatalf("Claims = %+v, %v", c, err)
	}
	if err := f.SignOutEverywhere(ctx, f.accounts.ada); err != nil || f.accounts.signedOut != f.accounts.ada {
		t.Fatalf("SignOutEverywhere = %v", err)
	}
}
