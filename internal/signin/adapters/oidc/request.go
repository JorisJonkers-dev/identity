package oidc

import (
	"slices"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"

	"github.com/JorisJonkers-dev/identity/internal/signin/domain"
)

// authRequest is a domain.AuthRequest as zitadel/oidc reads it.
type authRequest struct{ domain.AuthRequest }

var _ op.AuthRequest = authRequest{}

func (r authRequest) GetID() string         { return r.ID.String() }
func (authRequest) GetACR() string          { return "" }
func (r authRequest) GetAudience() []string { return []string{r.ClientID} }
func (r authRequest) GetClientID() string   { return r.ClientID }
func (r authRequest) GetNonce() string      { return r.Nonce }
func (r authRequest) GetRedirectURI() string {
	return r.RedirectURI
}

func (r authRequest) GetResponseType() oidc.ResponseType { return oidc.ResponseType(r.ResponseType) }

func (r authRequest) GetResponseMode() oidc.ResponseMode { return oidc.ResponseMode(r.ResponseMode) }
func (r authRequest) GetScopes() []string                { return r.Scopes }
func (r authRequest) GetState() string                   { return r.State }
func (r authRequest) Done() bool                         { return r.SignIn != nil }

// GetAMR is how the session was proved (RFC 8176), with "mfa" once that is more than one factor. A
// request completed before sessions recorded their methods was proved by a password.
func (r authRequest) GetAMR() []string {
	if r.SignIn == nil {
		return nil
	}
	if len(r.SignIn.Methods) == 0 {
		return []string{"pwd"}
	}
	amr := slices.Clone(r.SignIn.Methods)
	if len(amr) > 1 {
		amr = append(amr, "mfa")
	}
	return amr
}

func (r authRequest) GetAuthTime() time.Time {
	if r.SignIn == nil {
		return time.Time{}
	}
	return r.SignIn.At
}

func (r authRequest) GetSubject() string {
	if r.SignIn == nil {
		return ""
	}
	return r.SignIn.UserID.String()
}

func (r authRequest) GetCodeChallenge() *oidc.CodeChallenge {
	if r.CodeChallenge == "" {
		return nil
	}
	return &oidc.CodeChallenge{Challenge: r.CodeChallenge, Method: oidc.CodeChallengeMethod(r.CodeChallengeMethod)}
}
