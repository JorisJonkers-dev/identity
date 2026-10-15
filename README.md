# identity

The [JorisJonkers-dev](https://github.com/JorisJonkers-dev) estate's Identity Provider: one OIDC
sign-in for every app, at `https://auth.jorisjonkers.dev`. Apps register through a `client` block in
[deploy-kit](https://github.com/JorisJonkers-dev/deploy-kit) instead of code changes here; people
get access through Groups that hold App Roles; admins manage people and access in the portal. It
replaces auth-api and auth-ui
([workspace ADR-0020](https://github.com/JorisJonkers-dev/workspace/blob/main/docs/decisions/ADR-0020-identity-replaces-auth-api.md)).

**Status: foundations only.** The contract serves `GET /api/v1/version`, and the web app is a home
page showing it; sign-up and sign-in arrive with identity#11. The schema moves through
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
| `cmd/identity/` | The composition root; reads `DATABASE_URL` and `ADDR` (default `:8080`) |
| `web/` | The Vue 3 SPA, and `embed.go`, which embeds its build (`web/dist`) in the binary |
| `web/tests/e2e/` | Playwright with axe, on a desktop and a phone, against the built binary |
| `mise.toml`, `Taskfile.yml` | The pinned toolchain, and every command: `task` lists them |
| `Dockerfile` | Builds the web app, embeds it, ships a static binary on `distroless/static:nonroot` |
| `.github/workflows/ci.yml`, `.github/buckets.yml` | A `changes` job decides from the buckets file which jobs a diff reaches; one job per `task check` task, `task e2e` and each image build; `Pipeline Complete`, the one required check, passes when every job passed or was not reached |
| `.github/workflows/release.yml`, `release-please-config.json` | release-please, as in the rest of the estate |
| `deploy/identity.project.yml` | The [deploy-kit](https://github.com/JorisJonkers-dev/deploy-kit) Project Intent: one Process on the Issuer's host, a Postgres edge, the changelog. Not published until the cutover |

## The loop

```bash
mise install    # the pinned toolchain: Go, Node, pnpm, task, linters
task check      # lint, gen:check, Go and web tests with coverage, build, secret scan
task e2e        # builds, starts the compose Postgres, runs Playwright with axe
task dev        # Postgres, the API on :8080, Vite with hot reload on :5173
```

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
