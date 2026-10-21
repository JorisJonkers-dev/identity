# Flow-conformance suite

The Principal contract's suite for **native sign-in kits**: every language's kit signs people in
the same way, and this suite proves it. It runs a real identity in containers, then drives a minimal
app on the kit through four scenarios:

1. **A User signs in and the app reads their Principal.** The code flow with PKCE ends in a session
   whose Principal validates against `contract/principal`'s schema, stating `via: session` and the
   User's App Roles on the app.
2. **Signing out of identity ends the app's session.** identity sends a back-channel logout token,
   and the app stops knowing the User within five seconds (docs/adr/0007).
3. **A role taken away stops working in the app.** An Admin takes a role away, identity sends a
   back-channel logout (docs/adr/0023), and signing in again states only the roles still held.
4. **An action asking for fresh two-factor sends the browser to prove it.** The app asks identity
   for a proof with `acr_values=mfa` and `max_age` (docs/adr/0024). A User with no two-factor is
   sent to set it up, and once their session proves it, the action goes through.

## The app's routes

The app under test serves these in plain HTTP on `127.0.0.1:<Env.Port>`. The suite puts its own
HTTPS proxy in front of it at `Env.BaseURL`, because identity takes only https redirect URIs.

| Route | Answers |
|---|---|
| `GET /login?rd=<path>` | Starts a sign-in, coming back to `rd`, a path on the app. |
| `GET /callback` | The redirect URI: redeems the code, starts the app's session, and redirects to `rd`. |
| `POST /back-channel-logout` | Takes identity's logout token and ends the app sessions of its `sid`. |
| `GET /me` | 200 with the session's Principal as `contract/principal`'s schema states it, or 401 with no session. |
| `GET /fresh?minutes=<n>` | 200 with the Principal when the session proved two-factor within `n` minutes. Otherwise a redirect to identity asking for a fresh proof, coming back here. |

The app is told `Env`: the Issuer, its Client id (`flows-app`, a public PKCE Client holding the App
Roles `user` and `editor`), the base URL browsers use, and the port to listen on.

## Running it

From Go:

```go
func TestTheKitPassesThePrincipalContractsFlowSuite(t *testing.T) {
	flows.Run(t, flows.ImagesFromEnv(), func(t *testing.T, env flows.Env) {
		// Start the app on 127.0.0.1:env.Port, and stop it with t.Cleanup.
	})
}
```

`Images` names the identity server image and its migration image. `ImagesFromEnv` reads them from
`FLOWS_IDENTITY_IMAGE` and `FLOWS_MIGRATION_IMAGE`. Pin the identity release whose contract the kit
pins. Running needs Docker. Each run starts these on a network of its own:

- Postgres and the migration;
- a dev-mode Vault, with identity's transit keys;
- Mailpit, to read confirmation mail;
- identity, with the app's Client in `CLIENTS_FILE`.

identity reaches the app's back-channel logout endpoint through testcontainers' host access.

Each release is tagged `contract/flows/vX.Y.Z`:

```bash
go get github.com/JorisJonkers-dev/identity/contract/flows@vX.Y.Z
```

The Go kit in `JorisJonkers-dev/go-commons` (`identity/signin`) was the first to pass it. A kit in
another language starts its own app the same way, from a Go test or a small wrapper.
