package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var (
	// ErrInvalidMachineName is a name outside the rules.
	ErrInvalidMachineName = errors.New("a Machine Identity's name is 1 to 64 characters")
	// ErrInvalidBinding is a namespace or ServiceAccount Kubernetes would not accept.
	ErrInvalidBinding = errors.New("a namespace and a ServiceAccount are lower-case names Kubernetes accepts")
)

const maxMachineName = 64

// A namespace is a DNS label; a ServiceAccount a DNS subdomain.
var (
	namespacePattern      = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
	serviceAccountPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$`)
)

// Machine is a Machine Identity (docs/adr/0008): a workload that signs in as itself, bound to one
// Kubernetes ServiceAccount, holding Assignments directly and never in a Group.
type Machine struct {
	ID             uuid.UUID
	Name           string
	Namespace      string
	ServiceAccount string
	CreatedAt      time.Time
}

// NewMachine is a Machine Identity with a trimmed name of 1 to 64 characters, bound to a namespace
// and ServiceAccount Kubernetes accepts.
func NewMachine(id uuid.UUID, name, namespace, serviceAccount string, at time.Time) (Machine, error) {
	name = strings.TrimSpace(name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxMachineName {
		return Machine{}, ErrInvalidMachineName
	}
	if !namespacePattern.MatchString(namespace) || !serviceAccountPattern.MatchString(serviceAccount) {
		return Machine{}, ErrInvalidBinding
	}
	return Machine{ID: id, Name: name, Namespace: namespace, ServiceAccount: serviceAccount, CreatedAt: at}, nil
}

// MachineAssignment gives one Machine Identity one App Role on one Client.
type MachineAssignment struct {
	MachineID uuid.UUID
	ClientID  string
	Role      string
}
