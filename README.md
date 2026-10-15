# template-go-vue

A GitHub template repository for a contract-first Go + Vue application in the
[JorisJonkers-dev](https://github.com/JorisJonkers-dev) estate: a Go API generated from an OpenAPI
document with ogen, a Vue SPA whose client is generated from the same document with
`@hey-api/openapi-ts`, Postgres through goose and sqlc, and one distroless image in which the Go
binary serves the embedded web build. A repository generated from it builds, tests and runs its
end-to-end suite green with no edits.

The estate's decisions on this shape live in `JorisJonkers-dev/workspace`:
[ADR-0016](https://github.com/JorisJonkers-dev/workspace/blob/main/docs/decisions/ADR-0016-go-vue-contract-first-standard.md)
(Go + Vue, contract first) and
[ADR-0018](https://github.com/JorisJonkers-dev/workspace/blob/main/docs/decisions/ADR-0018-repository-templates.md)
(the repository templates). [`docs/blueprints/`](docs/blueprints/README.md) holds the architecture the
code follows, and says which parts the template leaves for later. A Go service without a web app
starts from [`template-go`](https://github.com/JorisJonkers-dev/template-go) instead. Hygiene files
(licence, security policy, `CODEOWNERS`, Renovate, editor config, release flow) come from
[`repo-template`](https://github.com/JorisJonkers-dev/repo-template).

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
| `internal/platform/` | Postgres (`pg`, with `pgtest` for a migrated database per test), `httpapi` (assembles the ogen server), `httpx`, `webui` |
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

## Use it

1. **Create the repository** with "Use this template" on GitHub.
2. **Rename the module.** Replace `github.com/JorisJonkers-dev/template-go-vue` with the new module
   path in `go.mod`, `.golangci.yml` (the goimports prefix and the depguard rule),
   `.testcoverage.yml` and every import.
3. **Rename the application.** Rename `cmd/template-go-vue/` to `cmd/<name>`, then replace
   `template-go-vue` in `Taskfile.yml` (`APP`), `Dockerfile`, `package.json`, `openapi/package.json`,
   `web/package.json`, `web/index.html`, `web/playwright.config.ts`, `openapi/v1/openapi.yaml`,
   `.github/workflows/ci.yml` and `release.yml`.
4. **Rename the project file.** Move `deploy/template-go-vue.project.yml` to
   `deploy/<name>.project.yml` and change `project`, `owner`, the Application `id`, the Process
   `name` and `image`, and the host.
5. **Reset release state**: delete `CHANGELOG.md` if present and keep `.release-please-manifest.json`
   at `0.0.0`.
6. `mise install && task check && task e2e`.

Ruleset and project boarding are described in [`CONTRIBUTING.md`](CONTRIBUTING.md) and
[`VERSIONING.md`](VERSIONING.md); `add-to-project.yml` should be deleted in a private repository,
as its header explains.
