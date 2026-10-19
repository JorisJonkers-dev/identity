package app_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/domain"
	audit "github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

func (f *fixture) only(t *testing.T) audit.Event {
	t.Helper()
	if len(*f.events) != 1 {
		t.Fatalf("%d events, want 1: %+v", len(*f.events), *f.events)
	}
	return (*f.events)[0]
}

func (f *fixture) nothing(t *testing.T) {
	t.Helper()
	if len(*f.events) != 0 {
		t.Fatalf("events = %+v", *f.events)
	}
}

var (
	adaInFamily = domain.Membership{GroupID: family.ID, UserID: ada}
	familyViews = domain.Assignment{GroupID: family.ID, ClientID: "grafana", Role: "viewer"}
)

func TestEachAccessChangeRecordsOneEventByTheAdmin(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before func(f *fixture)
		change func(f *fixture) error
		want   audit.Event
	}{
		{name: "making a Group", change: func(f *fixture) error {
			_, err := f.svc.CreateGroup(context.Background(), asAdmin, "Kin")
			return err
		}, want: audit.Event{Action: audit.GroupCreated, GroupName: "Kin"}},
		{name: "renaming a Group", change: func(f *fixture) error {
			_, err := f.svc.RenameGroup(context.Background(), asAdmin, family.ID, "Kin")
			return err
		}, want: audit.Event{Action: audit.GroupRenamed, Group: family.ID, GroupName: "Kin"}},
		{name: "deleting a Group", change: func(f *fixture) error {
			return f.svc.DeleteGroup(context.Background(), asAdmin, family.ID)
		}, want: audit.Event{Action: audit.GroupDeleted, Group: family.ID, GroupName: "Family"}},
		{name: "adding a member", change: func(f *fixture) error {
			return f.svc.AddMember(context.Background(), asAdmin, adaInFamily)
		}, want: audit.Event{Action: audit.MemberAdded, User: ada, Group: family.ID, GroupName: "Family"}},
		{name: "removing a member", before: func(f *fixture) { f.store.members = append(f.store.members, adaInFamily) }, change: func(f *fixture) error {
			return f.svc.RemoveMember(context.Background(), asAdmin, adaInFamily)
		}, want: audit.Event{Action: audit.MemberRemoved, User: ada, Group: family.ID, GroupName: "Family"}},
		{name: "giving an App Role", change: func(f *fixture) error {
			return f.svc.Assign(context.Background(), asAdmin, familyViews)
		}, want: audit.Event{Action: audit.RoleAssigned, Client: "grafana", Group: family.ID, GroupName: "Family", Role: "viewer"}},
		{name: "taking an App Role away", before: func(f *fixture) { f.store.assignments = append(f.store.assignments, familyViews) }, change: func(f *fixture) error {
			return f.svc.Unassign(context.Background(), asAdmin, familyViews)
		}, want: audit.Event{Action: audit.RoleRemoved, Client: "grafana", Group: family.ID, GroupName: "Family", Role: "viewer"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			if tc.before != nil {
				tc.before(f)
			}
			if err := tc.change(f); err != nil {
				t.Fatal(err)
			}
			e := f.only(t)
			want := tc.want
			want.Actor = admin
			if want.Action == audit.GroupCreated {
				want.Group = e.Group
			}
			if e != want || e.Group == uuid.Nil {
				t.Fatalf("event = %+v, want %+v", e, want)
			}
		})
	}
}

var adaViews = domain.UserAssignment{UserID: ada, ClientID: "grafana", Role: "viewer"}

func TestGivingAPersonAnAppRoleRecordsOneEventNamingThem(t *testing.T) {
	f := newFixture()
	if err := f.svc.AssignUser(context.Background(), asAdmin, adaViews); err != nil {
		t.Fatal(err)
	}
	if e := f.only(t); e != (audit.Event{Action: audit.RoleAssigned, Actor: admin, User: ada, Client: "grafana", Role: "viewer"}) {
		t.Fatalf("event = %+v", e)
	}
}

func TestTakingAPersonsAppRoleRecordsOneEventNamingThem(t *testing.T) {
	f := newFixture()
	f.store.direct = []domain.UserAssignment{adaViews}
	if err := f.svc.UnassignUser(context.Background(), asAdmin, adaViews); err != nil {
		t.Fatal(err)
	}
	if e := f.only(t); e != (audit.Event{Action: audit.RoleRemoved, Actor: admin, User: ada, Client: "grafana", Role: "viewer"}) {
		t.Fatalf("event = %+v", e)
	}
}

func TestAChangeThatChangesNothingRecordsNothing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(f *fixture) error
	}{
		{"adding a member twice", func(f *fixture) error {
			f.store.members = append(f.store.members, adaInFamily)
			return f.svc.AddMember(context.Background(), asAdmin, adaInFamily)
		}},
		{"removing someone who is not a member", func(f *fixture) error {
			return f.svc.RemoveMember(context.Background(), asAdmin, adaInFamily)
		}},
		{"giving an App Role twice", func(f *fixture) error {
			f.store.assignments = append(f.store.assignments, familyViews)
			return f.svc.Assign(context.Background(), asAdmin, familyViews)
		}},
		{"taking away an App Role the Group does not hold", func(f *fixture) error {
			return f.svc.Unassign(context.Background(), asAdmin, familyViews)
		}},
		{"giving a person an App Role twice", func(f *fixture) error {
			f.store.direct = append(f.store.direct, adaViews)
			return f.svc.AssignUser(context.Background(), asAdmin, adaViews)
		}},
		{"taking away an App Role the person does not hold", func(f *fixture) error {
			return f.svc.UnassignUser(context.Background(), asAdmin, adaViews)
		}},
		{"promoting an Admin again", func(f *fixture) error {
			return f.svc.Promote(context.Background(), admin, true)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			if err := tc.change(f); err != nil {
				t.Fatal(err)
			}
			f.nothing(t)
		})
	}
}

func TestAChangeTheStoreRefusesRecordsNothing(t *testing.T) {
	for _, tc := range []struct {
		method string
		change func(f *fixture) error
	}{
		{"CreateGroup", func(f *fixture) error {
			_, err := f.svc.CreateGroup(context.Background(), asAdmin, "Kin")
			return err
		}},
		{"RenameGroup", func(f *fixture) error {
			_, err := f.svc.RenameGroup(context.Background(), asAdmin, family.ID, "Kin")
			return err
		}},
		{"DeleteGroup", func(f *fixture) error { return f.svc.DeleteGroup(context.Background(), asAdmin, family.ID) }},
		{"AddMember", func(f *fixture) error { return f.svc.AddMember(context.Background(), asAdmin, adaInFamily) }},
		{"Assign", func(f *fixture) error { return f.svc.Assign(context.Background(), asAdmin, familyViews) }},
		{"AssignUser", func(f *fixture) error { return f.svc.AssignUser(context.Background(), asAdmin, adaViews) }},
		{"UnassignUser", func(f *fixture) error {
			f.store.direct = append(f.store.direct, adaViews)
			return f.svc.UnassignUser(context.Background(), asAdmin, adaViews)
		}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			f := newFixture()
			f.store.fail[tc.method] = errBoom
			if err := tc.change(f); err == nil {
				t.Fatal("the refusal was swallowed")
			}
			f.nothing(t)
		})
	}
}

func TestAMembershipChangeIsRecordedEvenWhenTheGroupCannotBeNamed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		action audit.Action
		change func(f *fixture) error
	}{
		{"removing a member", audit.MemberRemoved, func(f *fixture) error {
			f.store.members = append(f.store.members, adaInFamily)
			return f.svc.RemoveMember(context.Background(), asAdmin, adaInFamily)
		}},
		{"taking an App Role away", audit.RoleRemoved, func(f *fixture) error {
			f.store.assignments = append(f.store.assignments, familyViews)
			return f.svc.Unassign(context.Background(), asAdmin, familyViews)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture()
			f.store.fail["Group"] = errBoom
			if err := tc.change(f); err != nil {
				t.Fatal(err)
			}
			if e := f.only(t); e.Action != tc.action || e.GroupName != "" || e.Group != family.ID {
				t.Fatalf("event = %+v", e)
			}
		})
	}
}

func TestPromotingTheFirstAdminIsRecordedWithNoActor(t *testing.T) {
	f := newFixture()
	f.store.members = nil
	if err := f.svc.Promote(context.Background(), ada, false); err != nil {
		t.Fatal(err)
	}
	if e := f.only(t); e != (audit.Event{Action: audit.AdminPromoted, User: ada, Group: admins.ID, GroupName: "admins"}) {
		t.Fatalf("event = %+v", e)
	}
}

func TestSigningAUserOutAsksAsTheAdmin(t *testing.T) {
	f := newFixture()
	if err := f.svc.SignOutUser(context.Background(), asAdmin, ada); err != nil {
		t.Fatal(err)
	}
	if len(f.people.signedOutBy) != 1 || f.people.signedOutBy[0] != admin {
		t.Fatalf("signed out by %v", f.people.signedOutBy)
	}
}

func TestATwoFactorChangeIsOneEventByTheAdminNamingTheGroup(t *testing.T) {
	for _, tc := range []struct {
		required bool
		want     audit.Action
	}{{true, audit.GroupTwoFactorRequired}, {false, audit.GroupTwoFactorDropped}} {
		t.Run(string(tc.want), func(t *testing.T) {
			f := newFixture()
			_ = f.svc.SetTwoFactor(context.Background(), asAdmin, family.ID, tc.required)
			if e := f.only(t); e != (audit.Event{Action: tc.want, Actor: admin, Group: family.ID, GroupName: "Family"}) {
				t.Fatalf("event = %+v", e)
			}
		})
	}
}
