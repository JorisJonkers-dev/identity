package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JorisJonkers-dev/identity/internal/access/domain"
)

var made = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func TestAMachineIdentityKeepsItsTrimmedNameAndBinding(t *testing.T) {
	id := uuid.New()
	m, err := domain.NewMachine(id, "  Backup job ", "media", "backup.runner", made)
	if err != nil {
		t.Fatal(err)
	}
	if m != (domain.Machine{ID: id, Name: "Backup job", Namespace: "media", ServiceAccount: "backup.runner", CreatedAt: made}) {
		t.Fatalf("NewMachine = %+v", m)
	}
}

func TestAMachineIdentityNeedsANameOf1To64Characters(t *testing.T) {
	for _, name := range []string{" ", strings.Repeat("é", 65)} {
		if _, err := domain.NewMachine(uuid.New(), name, "media", "backup", made); !errors.Is(err, domain.ErrInvalidMachineName) {
			t.Errorf("NewMachine(%q) err = %v", name, err)
		}
	}
}

func TestAMachineIdentityNeedsANamespaceAndServiceAccountKubernetesAccepts(t *testing.T) {
	for _, binding := range []struct{ name, namespace, serviceAccount string }{
		{"an upper-case namespace", "Media", "backup"},
		{"a namespace with a dot", "me.dia", "backup"},
		{"an empty ServiceAccount", "media", ""},
		{"a ServiceAccount ending in a hyphen", "media", "backup-"},
	} {
		t.Run(binding.name, func(t *testing.T) {
			if _, err := domain.NewMachine(uuid.New(), "Backup", binding.namespace, binding.serviceAccount, made); !errors.Is(err, domain.ErrInvalidBinding) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
