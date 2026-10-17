---
status: accepted
---

# Personal Tokens are opaque, stored as a hash, and checked by introspection

A Personal Token (ADR 0008) is `idpat_` and 32 random bytes, shown to the User once. identity keeps only its SHA-256 hash. A Client that receives one checks it on each use at `/oidc/introspect` (RFC 7662), authenticating as itself, and learns the User and the App Roles that User holds on that Client now. A token the User revoked, one past its expiry, one for another Client, one for a Removed Client, and one whose User holds no App Role there any more all come back `active: false`.

zitadel/oidc serves the endpoint. It resolves an opaque token through the Provider's `Crypto`, and identity gives it a composite one: a decrypter that looks an `idpat_` token's hash up first, then zitadel's own AES decrypters for everything else. No protocol code is written here (ADR 0001), and no key has to outlive a restart or be shared between replicas.

## Considered Options

- **Exchange the Personal Token for a 15-minute JWT (RFC 8693).** Clients verify offline, as ADR 0014 has them do for access tokens, and a role taken away or a token revoked keeps working for up to 15 minutes, which the ticket's "immediately" rules out.
- **Opaque tokens sealed with zitadel's AES key.** Uses the library unchanged, and needs one key that survives restarts and replicas, whose loss forges any token.

## Consequences

- A Client calls identity once per request it does not cache. A Client may cache an answer briefly; the Gate does for 30 seconds (ADR 0009).
- Only a native Client with a secret can introspect. A PKCE Client has no secret, and the Client CRD gives a gated or token Client no credential at all. Those are checked by the Gate instead: on their hosts it accepts a Personal Token as a bearer token and runs the same check.
- The `Crypto` interface carries no context, so the hash lookup runs on its own two-second deadline.
