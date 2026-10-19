package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
	"github.com/JorisJonkers-dev/identity/internal/users/app"
)

// onDevices is Ada signed in on a Mac and then, a minute later, on a phone.
func onDevices(t *testing.T, opts ...app.Option) (f *fixture, id uuid.UUID, mac, phone string) {
	t.Helper()
	f = newFixture(opts...)
	id = f.signedUp(t, "ada@example.org")
	mac, _ = f.SignIn(withAgent("Firefox on macOS"), "ada@example.org", pw)
	f.now = t0.Add(time.Minute)
	phone, _ = f.SignIn(withAgent("Safari on iOS"), "ada@example.org", pw)
	return f, id, mac, phone
}

// withAgent is a context naming the device the user agent describes, as the web adapter names it.
func withAgent(label string) context.Context {
	return app.WithDevice(context.Background(), label)
}

func TestDevicesAreListedByBrowserAndOSTheMostRecentFirst(t *testing.T) {
	f, id, mac, _ := onDevices(t)
	got, err := f.Devices(context.Background(), id, mac)
	if err != nil || len(got) != 2 {
		t.Fatalf("Devices = %+v, %v", got, err)
	}
	if got[0].Label != "Safari on iOS" || got[0].Current || got[1].Label != "Firefox on macOS" || !got[1].Current || !got[1].LastSeenAt.Equal(t0) {
		t.Fatalf("Devices = %+v", got)
	}
}

func TestASessionStartedWithoutADeviceIsAnUnknownDevice(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	token, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	if got, _ := f.Devices(context.Background(), id, token); len(got) != 1 || got[0].Label != "Unknown device" {
		t.Fatalf("Devices = %+v", got)
	}
}

func TestSigningOutOneDeviceEndsOnlyThatSessionAndTellsItsApps(t *testing.T) {
	var told []uuid.UUID
	f, id, mac, phone := onDevices(t, app.WithSignedOut(func(_ context.Context, _ uuid.UUID, ids []uuid.UUID) { told = append(told, ids...) }))
	phoneID := f.store.sessions[sum(phone)].ID
	if err := f.SignOutDevice(context.Background(), id, phoneID); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.store.sessions[sum(phone)]; ok {
		t.Fatal("the phone is still signed in")
	}
	if _, ok := f.store.sessions[sum(mac)]; !ok || len(told) != 1 || told[0] != phoneID {
		t.Fatalf("mac kept %v, told %v", ok, told)
	}
}

func TestSigningOutADeviceIsAnAuditEvent(t *testing.T) {
	f, id, _, phone := onDevices(t)
	*f.events = nil
	_ = f.SignOutDevice(context.Background(), id, f.store.sessions[sum(phone)].ID)
	if len(*f.events) != 1 || (*f.events)[0].Action != audit.SignedOut || (*f.events)[0].User != id {
		t.Fatalf("events = %+v", *f.events)
	}
}

func TestSigningOutADeviceThatIsNotTheUsersIsNotFound(t *testing.T) {
	f, _, _, phone := onDevices(t)
	if err := f.SignOutDevice(context.Background(), uuid.New(), f.store.sessions[sum(phone)].ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestSigningOutEveryOtherDeviceKeepsThisOne(t *testing.T) {
	f, id, mac, phone := onDevices(t)
	*f.events = nil
	if err := f.SignOutOtherDevices(context.Background(), id, mac); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.store.sessions[sum(phone)]; ok || len(f.store.sessions) != 1 || len(*f.events) != 1 {
		t.Fatalf("sessions %d, events %+v", len(f.store.sessions), *f.events)
	}
}

func TestSigningOutOtherDevicesWhenThereAreNoneRecordsNothing(t *testing.T) {
	f := newFixture()
	id := f.signedUp(t, "ada@example.org")
	token, _ := f.SignIn(context.Background(), "ada@example.org", pw)
	*f.events = nil
	if err := f.SignOutOtherDevices(context.Background(), id, token); err != nil || len(*f.events) != 0 {
		t.Fatalf("err %v, events %+v", err, *f.events)
	}
}

func TestDevicesFailWhenTheStoreDoes(t *testing.T) {
	for _, tc := range []struct {
		method string
		call   func(f *fixture, id uuid.UUID, mac string) error
	}{
		{"SessionsOf", func(f *fixture, id uuid.UUID, mac string) error {
			_, err := f.Devices(context.Background(), id, mac)
			return err
		}},
		{"DeleteSession", func(f *fixture, id uuid.UUID, _ string) error {
			return f.SignOutDevice(context.Background(), id, uuid.New())
		}},
		{"DeleteOtherSessions", func(f *fixture, id uuid.UUID, mac string) error {
			return f.SignOutOtherDevices(context.Background(), id, mac)
		}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			f, id, mac, _ := onDevices(t)
			f.store.fail[tc.method] = errBoomSF
			if err := tc.call(f, id, mac); !errors.Is(err, errBoomSF) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
