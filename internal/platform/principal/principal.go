// Package principal carries who signed a request in, from the session security handler to every
// context's web adapter, so no adapter reads another's context keys.
package principal

import (
	"context"

	"github.com/google/uuid"
)

// Principal is the signed-in User, and whether their session proved two-factor.
type Principal struct {
	UserID    uuid.UUID
	TwoFactor bool
}

type key struct{}

// With returns ctx carrying p.
func With(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, key{}, p)
}

// From is the signed-in User, or the zero Principal when nobody is.
func From(ctx context.Context) Principal {
	p, _ := ctx.Value(key{}).(Principal)
	return p
}
