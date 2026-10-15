---
status: accepted
---

# identity signs with its own keys, rotated every 90 days, and imported secrets never leave the cluster

**Signing keys.** identity generates its own signing keys and does not import auth-api's. Tokens auth-api issued before the cutover stop being accepted by identity's Clients; they expire on auth-api's own short lifetimes, and a refresh sends the User through identity once. A new key starts signing every 90 days, and the previous key stays published in JWKS for 7 days more so that every token it signed can still be verified. Private keys are kept in Postgres encrypted with a Vault transit key, never in plaintext and never in an image.

**TOTP secrets** are stored encrypted with the same Vault transit service, under a key of their own. The one-off import (ADR 0006) runs as a Job inside the cluster: it reads auth-api's database directly, encrypts each secret through transit and writes it to identity's database, so no TOTP secret is ever exported, written to a file or moved outside the cluster.

**auth-api's service tokens are not migrated.** Their owners mint Personal Tokens in identity (ADR 0008). The old tokens keep working against Legacy Auth until it is retired.

Closes identity#8.

## Considered Options

- **Reuse auth-api's signing key and key id.** Tokens issued before the cutover would stay valid, at the cost of exporting a private key out of auth-api.
- **30-day rotation, or manual rotation only.** Tighter or simpler; 90 days with a 7-day overlap fits token lifetimes measured in minutes and hours.
- **An application-level encryption key from Vault.** Simpler than transit, but the key would live in identity's memory and environment.
- **Copy service tokens across as Personal Tokens.** Saves owners a step, and moves credentials whose holders nobody has re-confirmed.

## Consequences

- identity depends on Vault's transit engine at start-up (to unwrap signing keys) and for every TOTP check.
- The cutover signs every User out of every app once.
- Owners of auth-api service tokens must mint replacements before auth-api is retired.
