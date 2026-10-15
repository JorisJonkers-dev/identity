---
status: accepted
---

# Build the Identity Provider in Go on zitadel/oidc, with the Issuer at auth.jorisjonkers.dev

identity is our own Identity Provider, not an adopted product. It implements the storage interface of `zitadel/oidc` v3, which is OpenID Certified, so the protocol (authorization code with PKCE, discovery, JWKS, token exchange, back-channel logout) comes from that library and never from hand-written code here. The Issuer is `https://auth.jorisjonkers.dev`, the host auth-api serves today, so the OIDC clients that already exist keep their configuration through the migration. The estate-level decision is [workspace ADR-0020](https://github.com/JorisJonkers-dev/workspace/blob/main/docs/decisions/ADR-0020-identity-replaces-auth-api.md).

## Considered Options

- **Adopt Zitadel.** Go and certified, with an admin console, but Clients would be registered by hand or by a separate sync, never by deploy-kit.
- **Adopt Authentik.** Mature catalog and proxy outposts, but Python, heavier to run, and not driven by CRDs.
- **Another Issuer host** (`id.` or the apex). A clean break, but every existing client would be re-pointed at once, and the apex belongs to home-portal.

## Consequences

The Issuer is permanent: every Client pins it. Upgrades of `zitadel/oidc`, key rotation and security review are standing work for this repository.
