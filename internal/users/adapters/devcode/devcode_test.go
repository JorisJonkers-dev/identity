package devcode_test

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/JorisJonkers-dev/identity/internal/users/adapters/devcode"
)

func TestTheFixedCodeIsTakenAtAStepAfterTheLastOneUsed(t *testing.T) {
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	step, ok := devcode.Match("JBSWY3DPEHPK3PXP", devcode.Code, now, now.Unix()/30)
	if !ok || step <= now.Unix()/30 {
		t.Fatalf("%s = %d, %v; want a step after the last one used", devcode.Code, step, ok)
	}
}

func TestTheRealCodeIsStillTaken(t *testing.T) {
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	code, err := totp.GenerateCode("JBSWY3DPEHPK3PXP", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := devcode.Match("JBSWY3DPEHPK3PXP", code, now, 0); !ok {
		t.Fatal("the real code was refused")
	}
}

func TestAWrongCodeIsRefused(t *testing.T) {
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	if _, ok := devcode.Match("JBSWY3DPEHPK3PXP", "123456", now, 0); ok {
		t.Fatal("a wrong code was taken")
	}
}
