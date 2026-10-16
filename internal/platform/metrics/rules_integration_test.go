//go:build integration

package metrics_test

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"sigs.k8s.io/yaml"

	"github.com/JorisJonkers-dev/identity/internal/platform/metrics"
)

const promImage = "prom/prometheus:v3.5.0"

// TestTheAlertRulesPassPromtool runs deploy/alerts' promtool test against the PrometheusRule's own
// spec, so the rules the cluster loads are the rules tested.
func TestTheAlertRulesPassPromtool(t *testing.T) {
	raw, err := os.ReadFile("../../../deploy/alerts/identity.rules.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var rule struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec any `json:"spec"`
	}
	if err := yaml.Unmarshal(raw, &rule); err != nil {
		t.Fatal(err)
	}
	if rule.Metadata.Labels["release"] != "metrics-stack" {
		t.Fatal("without release: metrics-stack the operator never loads the rules")
	}
	spec, err := yaml.Marshal(rule.Spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(spec), metrics.Unassigned) {
		t.Fatalf("no rule reads %s", metrics.Unassigned)
	}
	cases, err := os.ReadFile("../../../deploy/alerts/identity.rules.test.yaml")
	if err != nil {
		t.Fatal(err)
	}

	c, err := testcontainers.Run(t.Context(), promImage,
		testcontainers.WithFiles(
			testcontainers.ContainerFile{Reader: strings.NewReader(string(spec)), ContainerFilePath: "/work/rules.yaml", FileMode: 0o644},
			testcontainers.ContainerFile{Reader: strings.NewReader(string(cases)), ContainerFilePath: "/work/test.yaml", FileMode: 0o644},
		),
		testcontainers.WithEntrypoint("promtool"),
		testcontainers.WithCmd("test", "rules", "/work/test.yaml"),
		testcontainers.WithWaitStrategy(wait.ForExit().WithExitTimeout(time.Minute)),
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	state, err := c.State(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.ExitCode != 0 {
		var out []byte
		if logs, err := c.Logs(t.Context()); err == nil {
			out, _ = io.ReadAll(logs)
		}
		t.Fatalf("promtool test rules exited %d:\n%s", state.ExitCode, out)
	}
}
