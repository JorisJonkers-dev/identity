---
status: accepted
---

# Access tokens are 15-minute JWTs, and no refresh tokens are issued yet

identity issues access tokens as JWTs signed with the same keys as ID tokens, valid for 15 minutes, so a Client verifies them against JWKS without calling identity. ID tokens are valid for an hour. No refresh tokens are issued: a Client that needs a new access token sends the browser through `/oidc/authorize` again, which completes without a password while the Single sign-on session holds. The Provider's endpoints live under `/oidc/`, with discovery at the Issuer's `/.well-known/openid-configuration`, so they never collide with the web app's paths.

## Considered Options

- **Opaque access tokens checked by introspection.** Revocable at once, at the cost of a call to identity on every request a Client serves.
- **Longer access tokens with refresh tokens.** Fewer round trips through `/authorize`, and a refresh token store to keep, rotate and revoke.

## Consequences

- Signing out everywhere (ADR 0007) ends the Single sign-on session and the Gate sessions at once, but an access token already issued keeps working for up to 15 minutes. Back-channel logout tells native Clients to drop their own sessions sooner.
- Token revocation is refused: there is nothing stored to revoke.
- Refresh tokens can be added later as their own decision, without changing the access token format.
