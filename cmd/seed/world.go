//go:build dev

package main

import (
	"time"

	signindomain "github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// password is every seeded person's password; it is printed when the seed ends.
const password = "correct horse battery staple"

// person is someone the canvas names. An authenticator app takes the dev code, 000000.
type person struct {
	name, email   string
	authenticator bool
}

var (
	joris  = person{"Joris Jonkers", "joris@example.org", true}
	ada    = person{"Ada Lovelace", "ada@example.org", true}
	femke  = person{"Femke de Boer", "femke@example.org", true}
	noor   = person{"Noor Bakker", "noor@example.org", false}
	sam    = person{"Sam de Vries", "sam@example.org", false}
	mila   = person{"Mila Jansen", "mila@example.org", false}
	pieter = person{"Pieter Bakker", "pieter@example.org", false}
	lotte  = person{"Lotte Bakker", "lotte@example.org", false}
	daan   = person{"Daan Jonkers", "daan@example.org", false}
	ruben  = person{"Ruben Visser", "ruben@example.org", false}
	iris   = person{"Iris Mulder", "iris@example.org", false}
	thijs  = person{"Thijs de Graaf", "thijs@example.org", false}
)

// people is everyone, in the order they signed up. Sam and Mila are in no Group: the dashboard's
// Unassigned queue.
var people = []person{joris, ada, femke, noor, sam, mila, pieter, lotte, daan, ruben, iris, thijs}

// longAgo is when the estate was set up: most people signed up then.
const longAgo = 60 * 24 * time.Hour

// signedUp is how long before now each person signed up: longAgo, but the two waiting for a Group
// this week, as the dashboard's Unassigned queue shows them on canvas frame C1.
func signedUp(p person) time.Duration {
	switch p.email {
	case sam.email:
		return 30 * time.Hour
	case mila.email:
		return 2 * time.Hour
	}
	return longAgo
}

// assignment is an App Role a Group holds on a Client.
type assignment struct{ client, role string }

// group is a Group of the canvas's frame C2, with its members and Assignments. Noor joins Family,
// and Family gets viewer on Grafana, within the last day, as canvas frame C4 lists them.
type group struct {
	name    string
	members []person
	roles   []assignment
}

var groups = []group{
	{
		name:    "Family",
		members: []person{joris, ada, pieter, lotte, daan},
		roles:   []assignment{{"grafana", "user"}, {"jellyfin", "user"}, {"grimoire", "user"}, {"tribelt-stats", "user"}, {"old-wiki", "user"}},
	},
	{
		name:    "Homelab ops",
		members: []person{ada, femke},
		roles:   []assignment{{"grafana", "user"}, {"grafana", "editor"}, {"vault", "user"}, {"vault", "operator"}, {"backup", "user"}, {"old-wiki", "user"}},
	},
	{
		name:    "Game night",
		members: []person{joris, daan, ruben, iris, thijs},
		roles:   []assignment{{"grimoire", "user"}, {"grimoire", "dm"}, {"jellyfin", "user"}, {"old-wiki", "user"}},
	},
}

// token is a Personal Token someone mints, ago before now, for days.
type token struct {
	owner          person
	name, clientID string
	days           int
	ago            time.Duration
	revoked        bool
}

// tokens are minted before the last day; grafana-cli and prometheus-scrape end within 7 days.
var tokens = []token{
	{owner: ada, name: "grafana-cli", clientID: "grafana", days: 30, ago: 25 * 24 * time.Hour},
	{owner: femke, name: "prometheus-scrape", clientID: "grafana", days: 30, ago: 28 * 24 * time.Hour},
	{owner: femke, name: "nightly-backup", clientID: "backup", days: 365, ago: 40 * 24 * time.Hour},
	{owner: joris, name: "jellyfin-sync", clientID: "jellyfin", days: 90, ago: 10 * 24 * time.Hour},
	{owner: ada, name: "old-laptop", clientID: "grafana", days: 90, ago: 20 * 24 * time.Hour, revoked: true},
}

// oldWiki is a Client the cluster no longer declares: it is not in dev/clients.yaml, so once the
// seed has given it Assignments it shows as a Removed app.
var oldWiki = signindomain.Client{
	ID: "old-wiki", SignIn: signindomain.Gated, Host: "wiki.localhost", Label: "Old wiki", Roles: []string{"user"},
}
