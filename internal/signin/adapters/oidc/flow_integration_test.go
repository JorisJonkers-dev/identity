//go:build integration

package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JorisJonkers-dev/identity/internal/access/adapters/catalog"
	accesspeople "github.com/JorisJonkers-dev/identity/internal/access/adapters/people"
	accesspersistence "github.com/JorisJonkers-dev/identity/internal/access/adapters/persistence"
	accessapp "github.com/JorisJonkers-dev/identity/internal/access/app"
	accessdomain "github.com/JorisJonkers-dev/identity/internal/access/domain"
	"github.com/JorisJonkers-dev/identity/internal/platform/passwords"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/queries"
	signinaccounts "github.com/JorisJonkers-dev/identity/internal/signin/adapters/accounts"
	clientindex "github.com/JorisJonkers-dev/identity/internal/signin/adapters/clients"
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

	e := serve(t, signinpersistence.New(q), signinaccounts.New(users), newAccess(), token)
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

// TestRolesComeFromAssignmentsOverPostgres signs an Admin in through real Groups and Assignments:
// being an Admin opens nothing, two Groups' roles add up in both tokens, no Group name leaks, and
// taking an Assignment away shows in the next token.
func TestRolesComeFromAssignmentsOverPostgres(t *testing.T) {
	ctx := context.Background()
	store, err := pg.Open(ctx, pgtest.URL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	q := store.Queries()

	mail := &inbox{}
	users := usersapp.New(userspersistence.New(q), passwords.Argon2id{}, mail, time.Now, rand.Reader, "https://auth.test")
	const pw = "correct horse battery"
	_ = users.SignUp(ctx, "ada@example.org", "Ada", pw)
	if err := users.Confirm(ctx, confirmLink.FindStringSubmatch(mail.last.Text)[1], pw); err != nil {
		t.Fatal(err)
	}
	token, err := users.SignIn(ctx, "ada@example.org", pw)
	if err != nil {
		t.Fatal(err)
	}
	ada, err := users.Authenticate(ctx, token)
	if err != nil {
		t.Fatal(err)
	}

	actor := accessapp.Actor{ID: ada.ID, SecondFactor: true}
	index := clientindex.NewIndex()
	index.Put(testClients["web"])
	access := accessapp.New(accesspersistence.New(q, store), catalog.New(index), accesspeople.New(users), discard{}, rand.Reader)
	groups, _ := q.ListGroups(ctx)
	if _, err := q.AddMember(ctx, queries.AddMemberParams{GroupID: groups[0].ID, UserID: ada.ID}); err != nil {
		t.Fatal(err)
	}

	e := serve(t, signinpersistence.New(q), signinaccounts.New(users), access, token)
	login := e.loginURL(t, authorizeQuery("web", "https://web.test/cb", ""))
	redirect(t, e.get(t, login, true), http.StatusSeeOther, "/no-access")

	viewers, err := access.CreateGroup(ctx, actor, "Viewers of things")
	if err != nil {
		t.Fatal(err)
	}
	editors, _ := access.CreateGroup(ctx, actor, "Editors of things")
	for _, a := range []accessdomain.Assignment{
		{GroupID: viewers.ID, ClientID: "web", Role: "viewer"},
		{GroupID: editors.ID, ClientID: "web", Role: "editor"},
	} {
		if err := access.Assign(ctx, actor, a); err != nil {
			t.Fatal(err)
		}
		if err := access.AddMember(ctx, actor, accessdomain.Membership{GroupID: a.GroupID, UserID: ada.ID}); err != nil {
			t.Fatal(err)
		}
	}

	signIn := func() tokens {
		t.Helper()
		code := e.code(t, authorizeQuery("web", "https://web.test/cb", ""))
		status, tok := e.exchange(t, url.Values{"code": {code}, "redirect_uri": {"https://web.test/cb"}}, "web", "s3cret")
		if status != http.StatusOK {
			t.Fatalf("token = %d %+v", status, tok)
		}
		return tok
	}
	// The session proved a Second Factor, and the ID token says so.
	sum := sha256.Sum256([]byte(token))
	if err := q.SetSessionMethods(ctx, queries.SetSessionMethodsParams{TokenHash: sum[:], Methods: []string{"pwd", "otp"}}); err != nil {
		t.Fatal(err)
	}
	tok := signIn()
	idToken := e.verify(t, "web", tok.IDToken)
	if amr := idToken.AuthenticationMethodsReferences; !slices.Equal(amr, []string{"pwd", "otp", "mfa"}) {
		t.Fatalf("amr = %v", amr)
	}
	want := []string{"viewer", "editor"}
	if got := roles(idToken.Claims["roles"]); !slices.Equal(got, want) {
		t.Fatalf("ID token roles = %v, want %v", got, want)
	}
	if got := roles(accessClaims(t, tok.AccessToken)["roles"]); !slices.Equal(got, want) {
		t.Fatalf("access token roles = %v, want %v", got, want)
	}
	for _, jwt := range []string{tok.IDToken, tok.AccessToken} {
		for _, part := range strings.Split(jwt, ".")[:2] {
			payload, _ := base64.RawURLEncoding.DecodeString(part)
			if strings.Contains(string(payload), "of things") || strings.Contains(string(payload), "admins") {
				t.Fatalf("a Group name is in a token: %s", payload)
			}
		}
	}

	if err := access.Unassign(ctx, actor, accessdomain.Assignment{GroupID: editors.ID, ClientID: "web", Role: "editor"}); err != nil {
		t.Fatal(err)
	}
	if got := roles(accessClaims(t, signIn().AccessToken)["roles"]); !slices.Equal(got, []string{"viewer"}) {
		t.Fatalf("roles after the Assignment went = %v", got)
	}
}
