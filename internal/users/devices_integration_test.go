//go:build integration

package users_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/platform/itest"
)

const (
	firefoxOnMac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14.6; rv:131.0) Gecko/20100101 Firefox/131.0"
	safariOnIOS  = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"
)

type deviceRow struct {
	ID, Label  string
	LastSeenAt time.Time
	Current    bool
}

func devices(t *testing.T, b *itest.Browser) []deviceRow {
	t.Helper()
	var d struct{ Devices []deviceRow }
	b.Call(t, http.MethodGet, "/api/v1/profile/devices", nil, http.StatusOK, &d)
	return d.Devices
}

func TestSecurityListsEachDeviceByBrowserAndOSAndMarksThisOne(t *testing.T) {
	t.Parallel()
	mac := itest.Start(t).SignedUp(t, "ada@example.org").SignedInAgain(t, firefoxOnMac)
	mac.SignedInAgain(t, safariOnIOS)
	got := devices(t, mac)
	if len(got) != 3 {
		t.Fatalf("devices = %+v", got)
	}
	var current []string
	labels := map[string]bool{}
	for _, d := range got {
		labels[d.Label] = true
		if d.Current {
			current = append(current, d.Label)
		}
	}
	if !labels["Firefox on macOS"] || !labels["Safari on iOS"] || len(current) != 1 || current[0] != "Firefox on macOS" {
		t.Fatalf("devices = %+v", got)
	}
}

func TestSigningOutOneDeviceEndsThatSessionAndLeavesTheOthers(t *testing.T) {
	t.Parallel()
	ada := itest.Start(t).SignedUp(t, "ada@example.org")
	phone := ada.SignedInAgain(t, safariOnIOS)
	laptop := ada.SignedInAgain(t, firefoxOnMac)
	var target string
	for _, d := range devices(t, laptop) {
		if d.Label == "Safari on iOS" {
			target = d.ID
		}
	}
	laptop.Call(t, http.MethodDelete, "/api/v1/profile/devices/"+target, nil, http.StatusNoContent, nil)
	phone.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, nil)
	laptop.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, nil)
}

func TestSigningOutEveryOtherDeviceKeepsOnlyThisOne(t *testing.T) {
	t.Parallel()
	ada := itest.Start(t).SignedUp(t, "ada@example.org")
	phone := ada.SignedInAgain(t, safariOnIOS)
	laptop := ada.SignedInAgain(t, firefoxOnMac)
	laptop.Call(t, http.MethodPost, "/api/v1/profile/other-devices/sign-out", nil, http.StatusNoContent, nil)
	phone.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
	ada.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusUnauthorized, nil)
	if got := devices(t, laptop); len(got) != 1 || !got[0].Current {
		t.Fatalf("devices = %+v", got)
	}
}

func TestSomeoneElsesDeviceIsNotFound(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	bob := env.SignedUp(t, "bob@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	bobs := devices(t, bob)[0].ID
	ada.Call(t, http.MethodDelete, "/api/v1/profile/devices/"+bobs, nil, http.StatusNotFound, nil)
	bob.Call(t, http.MethodGet, "/api/v1/profile", nil, http.StatusOK, nil)
}

func TestADeviceIsSeenAgainAtMostOnceAMinute(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	ada := env.SignedUp(t, "ada@example.org")
	first := devices(t, ada)[0].LastSeenAt
	env.Pass(30 * time.Second)
	if again := devices(t, ada)[0].LastSeenAt; !again.Equal(first) {
		t.Fatalf("seen at %v, then %v within a minute", first, again)
	}
	env.Pass(31 * time.Second)
	if later := devices(t, ada)[0].LastSeenAt; !later.After(first) {
		t.Fatalf("seen at %v, then %v after a minute", first, later)
	}
}

func TestSigningOutADeviceIsAnAuditEvent(t *testing.T) {
	t.Parallel()
	env := itest.Start(t)
	admin := env.Admin(t, "root@example.org")
	ada := env.SignedUp(t, "ada@example.org")
	ada.SignedInAgain(t, safariOnIOS)
	for _, d := range devices(t, ada) {
		if d.Label == "Safari on iOS" {
			ada.Call(t, http.MethodDelete, "/api/v1/profile/devices/"+d.ID, nil, http.StatusNoContent, nil)
		}
	}
	var page struct{ Events []struct{ Action string } }
	admin.Call(t, http.MethodGet, "/api/v1/audit?kind=sign-in&user="+admin.UserID(t, ada.Email), nil, http.StatusOK, &page)
	if len(page.Events) == 0 || page.Events[0].Action != "signed-out" {
		t.Fatalf("events = %+v", page.Events)
	}
}
