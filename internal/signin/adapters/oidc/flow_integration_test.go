//go:build integration

package oidc

import (
	"context"
	"crypto/rand"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/identity/internal/platform/passwords"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	signinaccounts "github.com/JorisJonkers-dev/identity/internal/signin/adapters/accounts"
	signinpersistence "github.com/JorisJonkers-dev/identity/internal/signin/adapters/persistence"
	userspersistence "github.com/JorisJonkers-dev/identity/internal/users/adapters/persistence"
	usersapp "github.com/JorisJonkers-dev/identity/internal/users/app"
)

type inbox struct{ last usersapp.Mail }

func (i *inbox) Send(_ context.Context, m usersapp.Mail) error {
	i.last = m
	return nil
}

var confirmLink = regexp.MustCompile(`confirm\?token=([A-Za-z0-9_-]+)`)

// TestASignedUpUserSignsInToAClientOverPostgres runs the flow over the real users and sign-in
// stores, so the ID token's sub and sid are the ones Postgres holds.
func TestASignedUpUserSignsInToAClientOverPostgres(t *testing.T) {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	q := queries.New(pool)

	mail := &inbox{}
	users := usersapp.New(userspersistence.New(q), passwords.Argon2id{}, mail, time.Now, rand.Reader, "https://auth.test")
	const pw = "correct horse battery"
	if err := users.SignUp(ctx, "ada@example.org", "Ada", pw); err != nil {
		t.Fatal(err)
	}
	if err := users.Confirm(ctx, confirmLink.FindStringSubmatch(mail.last.Text)[1], pw); err != nil {
		t.Fatal(err)
	}
	token, err := users.SignIn(ctx, "ada@example.org", pw)
	if err != nil {
		t.Fatal(err)
	}
	var userID, sessionID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT user_id, id FROM sessions`).Scan(&userID, &sessionID); err != nil {
		t.Fatal(err)
	}

	e := serve(t, signinpersistence.New(q), signinaccounts.New(users), token)
	code := e.code(t, authorizeQuery("web", "https://web.test/cb", ""))
	status, tok := e.exchange(t, url.Values{"code": {code}, "redirect_uri": {"https://web.test/cb"}}, "web", "s3cret")
	if status != 200 {
		t.Fatalf("token = %d %+v", status, tok)
	}
	c := e.verify(t, "web", tok.IDToken)
	if c.Subject != userID.String() || c.Claims["sid"] != sessionID.String() || c.Email != "ada@example.org" {
		t.Fatalf("ID token claims = %+v", c)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM auth_requests`).Scan(&left)
	if left != 0 {
		t.Fatalf("%d authorization requests left after the code was redeemed", left)
	}
}
