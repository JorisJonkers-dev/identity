//go:build integration

package main

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
	"github.com/JorisJonkers-dev/identity/internal/platform/pg/pgtest"
	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// The auth-api columns the import reads, as its Flyway migrations V1 to V8 leave them.
const authAPISchema = `
CREATE TABLE app_user (
    id UUID PRIMARY KEY,
    username VARCHAR(255) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    totp_secret VARCHAR(255),
    totp_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    role VARCHAR(50) NOT NULL DEFAULT 'USER',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    email_confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    first_name VARCHAR(100) NOT NULL DEFAULT '',
    last_name VARCHAR(100) NOT NULL DEFAULT ''
);
CREATE TABLE user_service_permissions (
    user_id UUID NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    service VARCHAR(50) NOT NULL,
    PRIMARY KEY (user_id, service)
);`

// legacyTOTP is the base32 secret auth-api keeps, unpadded, for an authenticator app.
const legacyTOTP = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"

var jellyfin = signindomain.Client{ID: "jellyfin", SignIn: signindomain.Gated, Host: "jellyfin.test", Label: "Jellyfin", Roles: []string{"user"}}

// legacyPerson is one auth-api row and its SERVICE_* grants.
type legacyPerson struct {
	email, first, last, role, password, totp string
	confirmed                                bool
	// unfinished is an authenticator app whose setup was never confirmed with a code.
	unfinished bool
	services   []string
}

// importWorld is a running identity beside an auth-api database holding people.
type importWorld struct {
	env    *itest.Env
	legacy *pgx.Conn
	vars   map[string]string
}

func newImportWorld(t *testing.T, people ...legacyPerson) *importWorld {
	t.Helper()
	env := itest.Start(t, jellyfin)
	legacyURL := pgtest.URL(t)
	conn, err := pgx.Connect(t.Context(), legacyURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })
	if _, err := conn.Exec(t.Context(), authAPISchema); err != nil {
		t.Fatal(err)
	}
	for _, p := range people {
		hash, err := bcrypt.GenerateFromPassword([]byte(p.password), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		id := uuid.New()
		var secret *string
		if p.totp != "" {
			secret = &p.totp
		}
		if _, err := conn.Exec(t.Context(), `INSERT INTO app_user (id, username, email, password_hash, totp_secret, totp_enabled, role, email_confirmed, first_name, last_name)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			id, strings.Split(p.email, "@")[0], p.email, string(hash), secret, p.totp != "" && !p.unfinished, p.role, p.confirmed, p.first, p.last); err != nil {
			t.Fatal(err)
		}
		for _, s := range p.services {
			if _, err := conn.Exec(t.Context(), `INSERT INTO user_service_permissions (user_id, service) VALUES ($1, $2)`, id, s); err != nil {
				t.Fatal(err)
			}
		}
	}
	addr, token := itest.Vault(t)
	return &importWorld{env: env, legacy: conn, vars: map[string]string{
		"DATABASE_URL": env.DatabaseURL(), "LEGACY_DATABASE_URL": legacyURL, "VAULT_ADDR": addr, "VAULT_TOKEN": token,
	}}
}

// run runs `identity import auth-api` with args and fails t unless it exits 0.
func (w *importWorld) run(t *testing.T, args ...string) string {
	t.Helper()
	var out, errs strings.Builder
	if code := command(t.Context(), env(w.vars), append([]string{"import", "auth-api"}, args...), &out, &errs); code != 0 {
		t.Fatalf("import = %d, %s", code, errs.String())
	}
	return out.String()
}

// rows counts what identity holds of people: Users, authenticator apps, direct Assignments, Admins.
func (w *importWorld) rows(t *testing.T) [4]int {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), w.env.DatabaseURL())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	var n [4]int
	for i, q := range []string{
		`SELECT count(*) FROM users`,
		`SELECT count(*) FROM totp_factors`,
		`SELECT count(*) FROM assignments WHERE user_id IS NOT NULL`,
		`SELECT count(*) FROM memberships m JOIN groups g ON g.id = m.group_id WHERE g.admins`,
	} {
		if err := conn.QueryRow(t.Context(), q).Scan(&n[i]); err != nil {
			t.Fatal(err)
		}
	}
	return n
}

// signIn signs in as email with password, answering the authenticator app's code when asked.
func (w *importWorld) signIn(t *testing.T, email, password string, want int) *itest.Browser {
	t.Helper()
	b := w.env.Browser()
	b.Call(t, http.MethodPost, "/api/v1/session", map[string]string{"email": email, "password": password}, want, nil)
	return b
}

var (
	ada   = legacyPerson{email: "ada@example.org", first: "Ada", last: "Lovelace", role: "USER", password: "an old bcrypt password", confirmed: true, services: []string{"JELLYFIN", "GRAFANA"}}
	grace = legacyPerson{email: "grace@example.org", first: "Grace", last: "Hopper", role: "ADMIN", password: "another old password", confirmed: true, totp: legacyTOTP}
)

func TestADryRunReportsWhoItWouldImportAndChangesNothing(t *testing.T) {
	w := newImportWorld(t, ada, grace)

	out := w.run(t, "--dry-run")

	for _, line := range []string{"would import ada@example.org", "would import grace@example.org", "Nothing was changed."} {
		if !strings.Contains(out, line) {
			t.Errorf("the dry run does not say %q:\n%s", line, out)
		}
	}
	if n := w.rows(t); n != [4]int{} {
		t.Fatalf("a dry run left Users, factors, Assignments and Admins %v", n)
	}
}

func TestAnImportedUserSignsInWithTheirOldPasswordAndIsRehashedWithArgon2id(t *testing.T) {
	w := newImportWorld(t, ada)
	w.run(t)

	w.signIn(t, ada.email, ada.password, http.StatusNoContent)

	var hash string
	conn, err := pgx.Connect(t.Context(), w.env.DatabaseURL())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(t.Context()) }()
	if err := conn.QueryRow(t.Context(), `SELECT password_hash FROM users WHERE email = $1`, ada.email).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("after signing in the hash is %.8s…, not argon2id", hash)
	}
	w.signIn(t, ada.email, ada.password, http.StatusNoContent)
}

func TestAnImportedUserIsRefusedAWrongPassword(t *testing.T) {
	w := newImportWorld(t, ada)
	w.run(t)

	w.signIn(t, ada.email, "not the old password", http.StatusUnauthorized)
}

func TestAnImportedAuthenticatorAppGivesValidCodes(t *testing.T) {
	w := newImportWorld(t, grace)
	w.run(t)
	b := w.signIn(t, grace.email, grace.password, http.StatusAccepted)

	code, err := totp.GenerateCode(legacyTOTP, w.env.Now())
	if err != nil {
		t.Fatal(err)
	}
	b.Call(t, http.MethodPost, "/api/v1/session/second-factor", map[string]string{"code": code}, http.StatusNoContent, nil)
}

func TestAnAuthenticatorAppWhoseSetupNeverFinishedIsNotImported(t *testing.T) {
	unfinished := grace
	unfinished.unfinished = true
	w := newImportWorld(t, unfinished)
	w.run(t)

	w.signIn(t, grace.email, grace.password, http.StatusNoContent)
}

func TestEachServiceGrantBecomesADirectUserRoleOnItsClient(t *testing.T) {
	w := newImportWorld(t, ada)
	w.run(t)
	b := w.signIn(t, ada.email, ada.password, http.StatusNoContent)

	var launcher struct{ Apps []struct{ ID string } }
	b.Call(t, http.MethodGet, "/api/v1/launcher", nil, http.StatusOK, &launcher)
	if len(launcher.Apps) != 1 || launcher.Apps[0].ID != jellyfin.ID {
		t.Fatalf("the Launcher shows %v, want Jellyfin", launcher.Apps)
	}
	if n := w.rows(t); n[2] != 2 {
		t.Fatalf("%d direct Assignments, want JELLYFIN and GRAFANA's", n[2])
	}
}

func TestAnAuthAPIAdminJoinsTheAdminsGroup(t *testing.T) {
	w := newImportWorld(t, ada, grace)

	w.run(t)

	if n := w.rows(t); n[3] != 1 {
		t.Fatalf("%d Admins, want grace", n[3])
	}
}

func TestAServiceMapsToTheClientNamedForIt(t *testing.T) {
	w := newImportWorld(t, legacyPerson{email: "lin@example.org", first: "Lin", role: "USER", password: "an old bcrypt password", confirmed: true, services: []string{"MEDIA"}})

	w.run(t, "--client", "MEDIA=jellyfin")

	b := w.signIn(t, "lin@example.org", "an old bcrypt password", http.StatusNoContent)
	var launcher struct{ Apps []struct{ ID string } }
	b.Call(t, http.MethodGet, "/api/v1/launcher", nil, http.StatusOK, &launcher)
	if !slices.ContainsFunc(launcher.Apps, func(a struct{ ID string }) bool { return a.ID == jellyfin.ID }) {
		t.Fatalf("the Launcher shows %v, want Jellyfin", launcher.Apps)
	}
}

func TestAnUnconfirmedEmailIsNotImported(t *testing.T) {
	w := newImportWorld(t, legacyPerson{email: "new@example.org", first: "New", role: "USER", password: "an old bcrypt password"})

	out := w.run(t)

	if !strings.Contains(out, "skip new@example.org: email not confirmed") || w.rows(t)[0] != 0 {
		t.Fatalf("an unconfirmed email was imported:\n%s", out)
	}
}

func TestAnEmailAlreadyInIdentityKeepsItsOwnAccount(t *testing.T) {
	w := newImportWorld(t, ada)
	b := w.env.SignedUp(t, "ada@example.org")
	if _, err := w.legacy.Exec(t.Context(), `UPDATE app_user SET email = $1`, b.Email); err != nil {
		t.Fatal(err)
	}

	out := w.run(t)

	if !strings.Contains(out, "skip "+b.Email+": email already in identity") {
		t.Fatalf("the import does not say the email is taken:\n%s", out)
	}
	w.signIn(t, b.Email, itest.Password, http.StatusNoContent)
}

func TestRunningTheImportTwiceChangesNothing(t *testing.T) {
	w := newImportWorld(t, ada, grace)
	w.run(t)
	first := w.rows(t)

	out := w.run(t)

	if n := w.rows(t); n != first {
		t.Fatalf("a second run moved %v to %v", first, n)
	}
	if !strings.Contains(out, "skip ada@example.org: already imported") {
		t.Fatalf("the second run does not say ada is already imported:\n%s", out)
	}
}
