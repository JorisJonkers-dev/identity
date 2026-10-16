---
status: accepted
---

# The Gate check is served from memory, with a p99 under 5 ms

The Gate check runs on every request to every gated app, so its cost multiplies. It must answer at p99 under 5 ms in the cluster with a warm cache; the token endpoint must answer at p99 under 50 ms. To get there, the host-to-Client index is in memory, Gate sessions live in Valkey behind a short-lived local cache, a session's last-seen time is written at most once a minute, and nothing is signed per request. auth-api's `/verify`, by contrast, reads and writes the session and signs a JWT on every call. Its p50 and p99 are taken from Traefik's access logs before identity serves, so the comparison is with a measured number, and a load test in CI fails when the target is missed.

## Until Valkey

The estate runs no Valkey yet, so Gate sessions live in Postgres behind the same local cache, which answers a check for 30 seconds. A sign-out or a role taken away reaches a gated host within that time. The load test (`TestTheGateCheckHoldsItsP99`) runs the real check over Postgres with a warm cache and fails at a p99 of 5 ms.

