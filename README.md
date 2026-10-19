# identity

The [JorisJonkers-dev](https://github.com/JorisJonkers-dev) estate's Identity Provider: one OIDC
sign-in for every app, at `https://auth.jorisjonkers.dev`. Apps register through a `client` block in
[deploy-kit](https://github.com/JorisJonkers-dev/deploy-kit) instead of code changes here; people
get access through Groups that hold App Roles; admins manage people and access in the portal. It
replaces auth-api and auth-ui
([workspace ADR-0020](https://github.com/JorisJonkers-dev/workspace/blob/main/docs/decisions/ADR-0020-identity-replaces-auth-api.md)).

**Status: sign-up, OIDC sign-in and access.** A person signs up, confirms their email from a mailed
link, signs in to a Single sign-on session and edits their Profile. The OIDC Provider serves
discovery at the Issuer and the code flow under `/oidc/`, on `zitadel/oidc`; a second app signs in
without a password while the session holds. Clients come from the cluster's `Client` objects,
watched without a restart; outside a cluster there are none. Admins put Users in Groups and give
Groups App Roles on Clients at `/access`; a User with no App Role on a Client gets no code, and the
ID token and access token carry that Client's App Roles, and nothing else about access, in a
`roles` claim. `identity admin promote <email>`, run in the pod, makes the first Admin, and the
access page lists the Users waiting for a Group. `identity import auth-api`, run once as a Job with
`LEGACY_DATABASE_URL` set, moves Legacy Auth's people over: confirmed Users with their bcrypt
hashes (rehashed with argon2id at their next sign-in), finished authenticator apps, each
`SERVICE_*` grant as a direct `user` role on the Client named like the service (`--client
MAIL=stalwart` names another), and ADMINs into the admins Group. `--dry-run` prints the same lines
and changes nothing; a second run skips everyone already imported
([0006](docs/adr/0006-legacy-auth-is-a-client-during-migration.md)). Users add authenticator apps (TOTP, sealed by Vault
transit) and passkeys (WebAuthn, on go-webauthn) on the Security page; sign-in then asks for one,
and Admins manage access only from a session that gave one
([0015](docs/adr/0015-a-second-factor-is-proved-not-just-held.md)). The Gate signs Users in for
gated apps one host at a time: Traefik's forwardAuth calls `/gate/check`, and a visitor without the
host's cookie gets a page with one sign-in button that comes back on the host's `/.auth/callback`. Kept signing keys and Client secrets arrive with
the following tickets. The schema moves through
liquibase-runner, not goose ([0010](docs/adr/0010-migrations-run-through-liquibase-runner.md)).

- [`CONTEXT.md`](CONTEXT.md): the words this repository uses, and the ones it avoids.
- [`docs/adr/`](docs/adr/): the decisions behind sign-in, access, sessions, tokens and migrations.
- deploy-kit [chapter 17](https://github.com/JorisJonkers-dev/deploy-kit/blob/main/spec/v1/17-sign-in.md):
  the `client` block and the `Client` object identity reads.

How the repository is laid out and worked on.

## What is in it

| Path | What it is |
|------|------------|
| `openapi/v1/openapi.yaml` | The contract, written first; both sides are generated from it |
| `openapi/` | Redocly `recommended-strict` and Spectral with the OWASP ruleset; vacuum runs beside them |
| `internal/platform/oas/` | The ogen server, generated (`go generate`) |
| `web/src/infrastructure/api/` | The TypeScript client, generated: types, a fetch SDK that zod-validates every response, vue-query options |
| `db/changelog/` | The Liquibase changelog liquibase-runner applies; every changeset rolls back |
| `db/schema.sql` | The schema the changelog produces, written by `task gen:schema`; sqlc reads it |
| `Dockerfile.migration` | The `identity-migration` image: liquibase-runner plus `db/changelog/` |
| `db/queries/`, `internal/platform/pg/queries/` | SQL, and the Go sqlc generates from it |
| `internal/platform/` | Postgres (`pg`, with `pgtest`: a database per test cloned from one liquibase-runner migrated, and the reversibility check), `httpapi` (assembles the ogen server), `httpx`, `webui` |
| `internal/server/` | Probes, `/api/` to the API, everything else to the SPA, graceful drain |
| `cmd/identity/` | The composition root; reads `DATABASE_URL`, `PUBLIC_URL`, `SMTP_ADDR`, `MAIL_FROM`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_TLS` and `ADDR` (default `:8080`) |
| `internal/users/` | The users context: sign-up, confirmation, sign-in, sessions and the Profile |
| `internal/platform/vault/` | Seals and opens TOTP secrets with Vault's transit engine, signing in with a token or the pod's service account |
| `internal/access/` | The access context: Groups, members and Assignments, managed by Admins, and the App Roles sign-in puts in tokens |
| `internal/gate/` | The Gate: the forwardAuth check, answered from a short-lived cache, the sign-in page, and the `/.auth/callback` that sets a host's own cookie |
| `internal/signin/` | The sign-in context: the OIDC Provider on zitadel/oidc, authorization requests, and the Single sign-on check at `/oidc/login` |
| `compose.yaml` | Postgres, the migration runner, Vault and Mailpit for `task dev` and `task e2e` |
| `dev/clients.yaml`, `cmd/seed` | The design canvas's apps, and the seed `task dev` runs; built only with the `dev` tag |
| `web/` | The Vue 3 SPA, and `embed.go`, which embeds its build (`web/dist`) in the binary |
| `web/tests/e2e/` | Playwright with axe, on a desktop and a phone, against the built binary |
| `mise.toml`, `Taskfile.yml` | The pinned toolchain, and every command: `task` lists them |
| `Dockerfile` | Builds the web app, embeds it, ships a static binary on `distroless/static:nonroot` |
| `.github/workflows/ci.yml`, `.github/buckets.yml` | A `changes` job decides from the buckets file which jobs a diff reaches; one job per `task check` task, `task e2e` and each image build; `Pipeline Complete`, the one required check, passes when every job passed or was not reached |
| `.github/workflows/release.yml`, `release-please-config.json` | release-please, as in the rest of the estate |
| `deploy/alerts/` | The `PrometheusRule` the observability project carries for identity, and its promtool test. identity serves `/metrics` on `METRICS_ADDR` (`:9090`), which the edge never routes |
| `deploy/crd/` | The cluster-scoped `Client` kind (`identity.jorisjonkers.dev/v1`) deploy-kit renders, with the CEL rules the API server holds every Client to. identity needs `get`, `list` and `watch` on `clients` |
| `deploy/identity.project.yml` | The [deploy-kit](https://github.com/JorisJonkers-dev/deploy-kit) Project Intent: one Process on the Issuer's host, a Postgres edge, the changelog. Not published until the cutover |

## The loop

```bash
mise install    # the pinned toolchain: Go, Node, pnpm, task, linters
task check      # lint, gen:check, Go and web tests with coverage, build, secret scan
task e2e        # builds, starts the compose Postgres, runs Playwright with axe
task dev        # identity at http://localhost:5173, seeded with the design canvas's people and apps
```

**Running it locally.** `task dev` starts the compose Postgres, Vault and Mailpit, migrates, seeds,
and serves the app at `http://localhost:5173` (Vite, hot reload) with the API on `:18480`. The
Clients are the design canvas's apps, from `dev/clients.yaml`. The seed (`cmd/seed`) adds the
canvas's people, Groups, Assignments, Personal Tokens and a day of Audit Events, through identity's
own API; it runs once and leaves a seeded database alone. Sign in as `joris@example.org`, an Admin,
with the password `correct horse battery staple` and the code `000000`; everyone else has the same
password. Mail lands in Mailpit at `http://localhost:28025`. `docker compose down -v` starts over.

**`000000` works only under `task dev`.** It is compiled in only with the `dev` build tag, which
`task dev` alone passes. Every other build, the release image among them, checks codes by RFC 6238
and refuses it; a test fails if a build file starts passing the tag.

`task check` needs Docker: the Go tests start Postgres through testcontainers. `go test -short`
skips the tests that need it.

**Changing the API** is always the same motion: edit `openapi/v1/openapi.yaml`, run `task gen` (which
lints the contract first), then fix what no longer compiles on either side. `task gen:check` fails
when committed generated code is stale, and CI runs it.

**Test buckets and coverage gates** ([0011](docs/adr/0011-unit-tests-reach-every-statement-branch-condition-and-function.md)).
`task test:unit` needs no Docker and holds Go and web to 100% of statements, branches and functions,
and Go also to 100% of conditions (gobco). `task test:integration` adds the tests behind the
`integration` build tag, which need Docker, and holds every function to at least one test. Totals
land in `coverage/coverage-totals.json`.

**Not behind forward-auth.** identity is the Identity Provider, so its host is `anonymous` at the
edge and its API authenticates with identity's own session (identity#11), never a header the edge
sets.
