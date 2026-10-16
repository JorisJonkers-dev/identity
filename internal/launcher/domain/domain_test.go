package domain_test

import (
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/launcher/domain"
)

func TestAClientWithALaunchURLOpensThere(t *testing.T) {
	c := domain.Client{Host: "grafana.test", LaunchURL: "https://grafana.test/dashboards"}
	if got := c.Launch(); got != "https://grafana.test/dashboards" {
		t.Fatalf("Launch() = %q", got)
	}
}

func TestAClientWithoutALaunchURLOpensAtItsHostsRoot(t *testing.T) {
	c := domain.Client{Host: "grafana.test"}
	if got := c.Launch(); got != "https://grafana.test/" {
		t.Fatalf("Launch() = %q", got)
	}
}
