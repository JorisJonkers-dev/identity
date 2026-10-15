package oidc

import (
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

func (r authRequest) GetAMR() []string {
	if r.SignIn == nil {
		return nil
	}
	return []string{"pwd"} // a Single sign-on session is proved by a password, for now
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
