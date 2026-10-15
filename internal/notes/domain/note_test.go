package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/JorisJonkers-dev/template-go-vue/internal/notes/domain"
)

func TestNewTextTrimsAndKeepsText(t *testing.T) {
	text, err := domain.NewText("  Buy milk \n")
	if err != nil {
		t.Fatal(err)
	}
	if text.String() != "Buy milk" {
		t.Fatalf("text = %q, want %q", text.String(), "Buy milk")
	}
}

func TestNewTextRefusesWhatTheDatabaseWould(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"only spaces":   " \t\n ",
		"one rune over": strings.Repeat("é", domain.MaxTextLength+1),
		"far too long":  strings.Repeat("x", 10*domain.MaxTextLength),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := domain.NewText(raw); !errors.Is(err, domain.ErrInvalidText) {
				t.Fatalf("NewText(%q) error = %v, want ErrInvalidText", raw, err)
			}
		})
	}
}

func TestNewTextCountsRunesNotBytes(t *testing.T) {
	if _, err := domain.NewText(strings.Repeat("é", domain.MaxTextLength)); err != nil {
		t.Fatalf("%d two-byte runes refused: %v", domain.MaxTextLength, err)
	}
}
