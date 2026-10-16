// Package principal carries who signed a request in, from the session security handler to every
// context's web adapter, so no adapter reads another's context keys.
package principal

import (
	"context"

	"github.com/google/uuid"
)

type key struct{}

// With returns ctx carrying the signed-in User.
func With(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, key{}, userID)
}

// From is the signed-in User, or uuid.Nil when nobody is.
func From(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(key{}).(uuid.UUID)
	return id
}
