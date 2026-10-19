package app_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/legacy/app"
	"github.com/JorisJonkers-dev/identity/internal/legacy/domain"
	usersdomain "github.com/JorisJonkers-dev/identity/internal/users/domain"
)

var signedUp = time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC)

// account is a confirmed Legacy Auth account, for a test to change what it is about.
func account(email string) domain.Account {
	return domain.Account{
		ID: uuid.New(), Username: strings.Split(email, "@")[0], Email: email, FirstName: "Ada", LastName: "Lovelace",
		PasswordHash: "$2a$10$legacy", Confirmed: true, CreatedAt: signedUp,
	}
}

type source struct {
	accounts []domain.Account
	fail     error
}

func (s *source) Accounts(context.Context) ([]domain.Account, error) { return s.accounts, s.fail }

// target is identity holding the people imported, and the emails others signed up with.
type target struct {
	people     map[uuid.UUID]app.Person
	taken      []string
	raced      bool
	failState  error
	failImport error
}

func (t *target) State(_ context.Context, id uuid.UUID, email usersdomain.Email) (app.State, error) {
	_, imported := t.people[id]
	return app.State{Imported: imported, Taken: slices.Contains(t.taken, email.String())}, t.failState
}

func (t *target) Import(_ context.Context, p app.Person) (bool, error) {
	if t.failImport != nil || t.raced {
		return false, t.failImport
	}
	t.people[p.ID] = p
	return true, nil
}

// sealer "seals" by prefixing, as Vault's ciphertexts are.
type sealer struct{ fail error }

func (s sealer) Seal(_ context.Context, plaintext []byte) (string, error) {
	return "vault:v1:" + string(plaintext), s.fail
}

type world struct {
	source *source
	target *target
	sealer *sealer
}

func newWorld(accounts ...domain.Account) *world {
	return &world{source: &source{accounts: accounts}, target: &target{people: map[uuid.UUID]app.Person{}}, sealer: &sealer{}}
}

func (w *world) run(t *testing.T, dryRun bool) []app.Outcome {
	t.Helper()
	outcomes, err := app.New(w.source, w.target, w.sealer, map[string]string{"MAIL": "stalwart"}).Run(context.Background(), dryRun)
	if err != nil {
		t.Fatal(err)
	}
	return outcomes
}

func TestADryRunSaysWhatItWouldImportAndImportsNothing(t *testing.T) {
	a := account("ada@example.org")
	a.Admin, a.TOTPSecret, a.Services = true, "JBSWY3DPEHPK3PXP", []string{"GRAFANA"}
	w := newWorld(a)

	outcomes := w.run(t, true)

	want := app.Outcome{Email: "ada@example.org", Admin: true, Authenticator: true, Clients: []string{"grafana"}}
	if len(outcomes) != 1 || !equal(outcomes[0], want) || len(w.target.people) != 0 {
		t.Fatalf("a dry run = %+v and imported %d", outcomes, len(w.target.people))
	}
}

func TestAnAccountIsImportedWithItsOwnIdHashNameAndSignUpTime(t *testing.T) {
	a := account("Ada@Example.org")
	w := newWorld(a)

	w.run(t, false)

	p := w.target.people[a.ID]
	if p.Email.String() != "ada@example.org" || p.Name.String() != "Ada Lovelace" || p.PasswordHash != a.PasswordHash || !p.CreatedAt.Equal(signedUp) {
		t.Fatalf("imported %+v", p)
	}
}

func TestEachGrantBecomesARoleOnTheClientForItsServiceOnce(t *testing.T) {
	a := account("ada@example.org")
	a.Services = []string{"MAIL", "KNOWLEDGE_API", "STALWART"}
	w := newWorld(a)

	w.run(t, false)

	if got := w.target.people[a.ID].Clients; !slices.Equal(got, []string{"knowledge-api", "stalwart"}) {
		t.Fatalf("Clients = %v", got)
	}
}

func TestAnAdminIsImportedAsAnAdmin(t *testing.T) {
	a := account("ada@example.org")
	a.Admin = true
	w := newWorld(a)

	w.run(t, false)

	if !w.target.people[a.ID].Admin {
		t.Fatal("an auth-api ADMIN was imported as no Admin")
	}
}

func TestAnAuthenticatorSecretIsImportedSealed(t *testing.T) {
	a := account("ada@example.org")
	a.TOTPSecret = "JBSWY3DPEHPK3PXP"
	w := newWorld(a)

	w.run(t, false)

	if got := w.target.people[a.ID].SealedTOTP; got != "vault:v1:JBSWY3DPEHPK3PXP" {
		t.Fatalf("SealedTOTP = %q", got)
	}
}

func TestAnAccountWithoutAnAuthenticatorImportsNoSecret(t *testing.T) {
	a := account("ada@example.org")
	w := newWorld(a)
	w.sealer.fail = errors.New("sealed something")

	w.run(t, false)

	if got := w.target.people[a.ID].SealedTOTP; got != "" {
		t.Fatalf("SealedTOTP = %q", got)
	}
}

func TestAnAccountIsSkippedWhenItCannotBeImported(t *testing.T) {
	taken := account("taken@example.org")
	imported := account("again@example.org")
	cases := []struct {
		name string
		edit func(*domain.Account)
		want string
	}{
		{"unconfirmed", func(a *domain.Account) { a.Confirmed = false }, app.SkipUnconfirmed},
		{"not an email", func(a *domain.Account) { a.Email = "ada" }, app.SkipBadEmail},
		{"no usable name", func(a *domain.Account) { a.FirstName, a.LastName, a.Username = "", "", "" }, app.SkipBadName},
		{"already imported", func(a *domain.Account) { *a = imported }, app.SkipImported},
		{"email taken", func(a *domain.Account) { *a = taken }, app.SkipTaken},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := account("ada@example.org")
			c.edit(&a)
			w := newWorld(a)
			w.target.taken = []string{"taken@example.org"}
			w.target.people[imported.ID] = app.Person{ID: imported.ID}

			outcomes := w.run(t, false)

			if len(outcomes) != 1 || outcomes[0].Skipped != c.want || len(w.target.people) != 1 {
				t.Fatalf("outcomes = %+v, %d people", outcomes, len(w.target.people))
			}
		})
	}
}

func TestAnAccountImportedMeanwhileIsSkipped(t *testing.T) {
	w := newWorld(account("ada@example.org"))
	w.target.raced = true

	outcomes := w.run(t, false)

	if outcomes[0].Skipped != app.SkipImported {
		t.Fatalf("outcome = %+v", outcomes[0])
	}
}

func TestTheImportStopsAtAFailure(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name  string
		spoil func(*world)
	}{
		{"reading auth-api", func(w *world) { w.source.fail = boom }},
		{"reading identity", func(w *world) { w.target.failState = boom }},
		{"sealing", func(w *world) { w.sealer.fail = boom }},
		{"writing identity", func(w *world) { w.target.failImport = boom }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := account("ada@example.org")
			a.TOTPSecret = "JBSWY3DPEHPK3PXP"
			w := newWorld(a)
			c.spoil(w)

			_, err := app.New(w.source, w.target, w.sealer, nil).Run(context.Background(), false)

			if !errors.Is(err, boom) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func equal(a, b app.Outcome) bool {
	return a.Email == b.Email && a.Skipped == b.Skipped && a.Admin == b.Admin && a.Authenticator == b.Authenticator && slices.Equal(a.Clients, b.Clients)
}
