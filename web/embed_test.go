package web

import (
	"io/fs"
	"testing"
)

func TestDistServesTheBuildDirectory(t *testing.T) {
	if _, err := fs.Stat(Dist(), ".gitkeep"); err != nil {
		t.Fatalf("Dist has no .gitkeep: %v", err)
	}
}

func TestSubPanicsOnAnInvalidDirectory(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("sub with an invalid directory did not panic")
		}
	}()
	sub(dist, "../outside")
}
