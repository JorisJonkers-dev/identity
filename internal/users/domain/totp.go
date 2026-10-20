package domain

import (
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
)

// ErrInvalidLabel is a two-factor method name outside the rules.
var ErrInvalidLabel = errors.New("a name for a two-factor method is 1 to 64 characters")

const (
	totpIssuer    = "jorisjonkers.dev"
	totpPeriod    = 30
	totpDigits    = 6
	totpSecretLen = 20
	maxLabel      = 64
)

// FactorLabel is what a User calls one of their two-factor methods, such as "Phone".
type FactorLabel struct{ value string }

// NewFactorLabel trims raw and bounds it by characters.
func NewFactorLabel(raw string) (FactorLabel, error) {
	value := strings.TrimSpace(raw)
	if n := utf8.RuneCountInString(value); n < 1 || n > maxLabel {
		return FactorLabel{}, ErrInvalidLabel
	}
	return FactorLabel{value: value}, nil
}

func (l FactorLabel) String() string { return l.value }

// NewTOTPSecret is 160 random bits in unpadded base32, as authenticator apps expect.
func NewTOTPSecret(random io.Reader) (string, error) {
	b := make([]byte, totpSecretLen)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

// TOTPURI is the otpauth URI an authenticator app reads from a QR code.
func TOTPURI(secret, account string) string {
	q := url.Values{
		"secret": {secret}, "issuer": {totpIssuer}, "algorithm": {"SHA1"},
		"digits": {fmt.Sprint(totpDigits)}, "period": {fmt.Sprint(totpPeriod)},
	}
	return "otpauth://totp/" + url.PathEscape(totpIssuer+":"+account) + "?" + q.Encode()
}

// MatchTOTP checks a six-digit code against the 30-second step at now and the steps either side,
// accepting only steps after the last one accepted, so no code works twice. It returns the step the
// code was for.
func MatchTOTP(secret, code string, now time.Time, after int64) (int64, bool) {
	if len(code) != totpDigits || strings.Trim(code, "0123456789") != "" {
		return 0, false
	}
	current := now.Unix() / totpPeriod
	for step := current - 1; step <= current+1; step++ {
		if step <= after {
			continue
		}
		want, err := hotp.GenerateCodeCustom(secret, uint64(step), hotp.ValidateOpts{Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}) //nolint:gosec // steps are positive Unix half-minutes
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}
