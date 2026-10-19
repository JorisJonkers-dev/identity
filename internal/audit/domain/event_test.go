package domain_test

import (
	"testing"
	"time"

	"github.com/JorisJonkers-dev/identity/internal/audit/domain"
)

func TestEachActionBelongsToItsKind(t *testing.T) {
	t.Parallel()
	for kind, actions := range map[domain.Kind][]domain.Action{
		domain.KindSignIn: {domain.SignedIn, domain.SignInFailed, domain.SignedOut},
		domain.KindPeople: {domain.UserCreated, domain.UserRenamed, domain.PasswordChanged, domain.SecondFactorAdded, domain.SecondFactorRemoved, domain.AdminPromoted},
		domain.KindAccess: {domain.GroupCreated, domain.GroupRenamed, domain.GroupDeleted, domain.MemberAdded, domain.MemberRemoved, domain.RoleAssigned, domain.RoleRemoved, domain.ClientSecretRotated, domain.StepUpChanged},
		domain.KindTokens: {domain.TokenMinted, domain.TokenRevoked},
	} {
		for _, a := range actions {
			t.Run(string(a), func(t *testing.T) {
				t.Parallel()
				if got := a.Kind(); got != kind {
					t.Fatalf("%s is %q, want %q", a, got, kind)
				}
			})
		}
	}
}

func TestAnUnknownActionHasNoKind(t *testing.T) {
	t.Parallel()
	if got := domain.Action("teleported").Kind(); got != "" {
		t.Fatalf("kind = %q", got)
	}
}

func TestAZeroCursorIsTheStartOfTheListing(t *testing.T) {
	t.Parallel()
	if !(domain.Cursor{}).Zero() || (domain.Cursor{At: time.Unix(1, 0)}).Zero() {
		t.Fatal("only a cursor with no time is the start")
	}
}
