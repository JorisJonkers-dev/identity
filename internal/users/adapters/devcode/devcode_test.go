//go:build !dev

package devcode_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/identity/internal/users/adapters/devcode"
)

func TestAReleaseBuildTakesNoFixedSecondFactorCode(t *testing.T) {
	if devcode.TOTPMatch() != nil {
		t.Fatal("a build without the dev tag replaces the authenticator code check")
	}
}

// devTag is a go build or go test flag naming the dev tag.
var devTag = regexp.MustCompile(`-tags[= ]\S*\bdev\b`)

func TestNoReleaseBuildPassesTheDevTag(t *testing.T) {
	files, err := filepath.Glob("../../../../.github/workflows/*.y*ml")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range append(files, "../../../../Dockerfile", "../../../../Dockerfile.migration", "../../../../scripts/coverage.sh") {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path) //nolint:gosec // the repository's own build files
			if err != nil {
				t.Fatal(err)
			}
			if devTag.Match(raw) {
				t.Fatalf("%s builds with the dev tag", path)
			}
		})
	}
	raw, err := os.ReadFile("../../../../Taskfile.yml")
	if err != nil {
		t.Fatal(err)
	}
	// Only the dev task and the dev-tag test step may name it.
	for _, line := range strings.Split(string(raw), "\n") {
		if devTag.MatchString(line) && !strings.Contains(line, "# dev only") {
			t.Fatalf("Taskfile.yml builds with the dev tag outside the dev task: %q", strings.TrimSpace(line))
		}
	}
}
