---
status: accepted
---

# The Gate signs a Principal once per cached check

Every allowed Gate check now carries `X-Identity-Principal`: the Principal contract's facts (contract/principal), signed with identity's key ring, typed `identity-principal+jwt`, bound to the Client as audience and valid for 60 seconds. Our own apps read only that header, so a header a caller supplies can never become their User. The plain `X-Auth-Request-*` headers stay for third-party gated apps, and the Gate sets every one of them on every allowed answer, empty when it has no value.

ADR 0009 kept the check in memory, with nothing signed per request. The Gate signs the Principal when it computes a check's answer, caches it with that answer, and keeps the answer for at most half the Principal's lifetime. A warm check still signs nothing, and an app always receives a Principal with at least 30 seconds left.

## Considered Options

- **Sign on every request.** Each Principal is fresh, but an RS256 signature on every check spends a large part of the 5 ms p99 budget.
- **An unsigned header the edge must strip.** Nothing to sign, but an app is then only as safe as the edge's configuration, and one missed route lets a caller choose their own User.
- **Cache the signed Principal for the cache's full ttl.** Fewer signatures, but an app could receive a Principal moments before it expires.

## Consequences

- The Gate's cache entries now last at most 30 seconds whatever the cache's ttl, so a sign-out or a role taken away still reaches a gated host within that time.
- A Principal's `iat` is when the check was computed, not when the request arrived. An app must not read `iat` as the request's time.
- The signed header depends on the key ring (ADR 0013): a rotated key signs the next computed check, and JWKS publishes both keys while Principals signed with the old one are alive.
