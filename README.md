# identity

The [JorisJonkers-dev](https://github.com/JorisJonkers-dev) estate's Identity Provider: one OIDC
sign-in for every app, at `https://auth.jorisjonkers.dev`. Apps register through a `client` block in
[deploy-kit](https://github.com/JorisJonkers-dev/deploy-kit) instead of code changes here; people
get access through Groups that hold App Roles; admins manage people and access in the portal. It
replaces auth-api and auth-ui
([workspace ADR-0020](https://github.com/JorisJonkers-dev/workspace/blob/main/docs/decisions/ADR-0020-identity-replaces-auth-api.md)).

**Status: bootstrapped from `template-go-vue`.** The sample `notes` context below is still the
template's and is replaced as the first features land. Unlike the template, the schema moves through
liquibase-runner, not goose ([0010](docs/adr/0010-migrations-run-through-liquibase-runner.md)).

- [`CONTEXT.md`](CONTEXT.md): the words this repository uses, and the ones it avoids.
- [`docs/adr/`](docs/adr/): the decisions behind sign-in, access, sessions, tokens and migrations.
- deploy-kit [chapter 17](https://github.com/JorisJonkers-dev/deploy-kit/blob/main/spec/v1/17-sign-in.md):
  the `client` block and the `Client` object identity reads.

The template's own notes follow.

## What is in it

| Path | What it is |
|------|------------|
| `openapi/v1/openapi.yaml` | The contract, written first; both sides are generated from it |
| `openapi/` | Redocly `recommended-strict` and Spectral with the OWASP ruleset; vacuum runs beside them |
| `internal/platform/oas/` | The ogen server, generated (`go generate`) |
| `web/src/infrastructure/api/` | The TypeScript client, generated: types, a fetch SDK that zod-validates every response, vue-query options |
| `db/migrations/` | goose migrations, embedded and applied by the binary at startup; linted by squawk |
| `db/queries/`, `internal/platform/pg/queries/` | SQL, and the Go sqlc generates from it |
| `internal/notes/` | The sample bounded context: `domain` (pure), `app` (use cases), `adapters/persistence` and `adapters/web` |
| `internal/platform/` | Postgres (`pg`, with `pgtest`: a database per test cloned from one liquibase-runner migrated, and the reversibility check), `httpapi` (assembles the ogen server), `httpx`, `webui` |
| `internal/server/` | Probes, `/api/` to the API, everything else to the SPA, graceful drain |
| `cmd/template-go-vue/` | The composition root; reads `DATABASE_URL`, `ADDR` (default `:8080`) and `DEV_USER` |
| `web/` | The Vue 3 SPA, and `embed.go`, which embeds its build (`web/dist`) in the binary |
| `web/tests/e2e/` | Playwright with axe, on a desktop and a phone, against the built binary |
| `mise.toml`, `Taskfile.yml` | The pinned toolchain, and every command: `task` lists them |
| `Dockerfile` | Builds the web app, embeds it, ships a static binary on `distroless/static:nonroot` |
| `.github/workflows/ci.yml` | One parallel job per `task check` task, one for `task e2e` and one for `docker build`; `Pipeline Complete`, the one required check, passes when all do |
| `.github/workflows/release.yml`, `release-please-config.json` | release-please, as in the rest of the estate |
| `deploy/template-go-vue.project.yml` | A [deploy-kit](https://github.com/JorisJonkers-dev/deploy-kit) Project Intent: one Process, an authenticated host, a Postgres edge |

The sample resource is a list of notes, there only to show every layer once. Replace it.

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

**Coverage gates.** Go: 80% total and 100% on each context's `domain` and `app`
(`.testcoverage.yml`), measured across packages and excluding generated code. Web: 90% on lines,
branches, functions and statements (`web/vite.config.ts`). Raise them as the suite grows.

**Identity.** The API requires an `X-User-Id` header, which the platform's forward-auth sets at the
edge (`audience: authenticated` in the project file). On a local run nothing sits in front of the
binary, so `DEV_USER` fills the header in. Never set `DEV_USER` in a deployment.
