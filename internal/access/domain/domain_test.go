package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/access/domain"
)

func TestNewGroupName(t *testing.T) {
	for _, tc := range []struct {
		raw, want string
		err       error
	}{
		{raw: "  Family ", want: "Family"},
		{raw: strings.Repeat("é", 64), want: strings.Repeat("é", 64)},
		{raw: "x", want: "x"},
		{raw: "   ", err: domain.ErrInvalidGroupName},
		{raw: strings.Repeat("é", 65), err: domain.ErrInvalidGroupName},
	} {
		got, err := domain.NewGroupName(tc.raw)
		if !errors.Is(err, tc.err) || got.String() != tc.want {
			t.Errorf("NewGroupName(%q) = %q, %v; want %q, %v", tc.raw, got, err, tc.want, tc.err)
		}
	}
}

func TestAClientDeclaresOnlyItsOwnAppRoles(t *testing.T) {
	c := domain.Client{ID: "grafana", Roles: []string{"user", "editor"}}
	if !c.Declares("editor") || c.Declares("admin") {
		t.Fatalf("Declares: editor %v, admin %v", c.Declares("editor"), c.Declares("admin"))
	}
}
