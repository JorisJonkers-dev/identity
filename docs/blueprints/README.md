# Blueprints

Generic, product-agnostic architecture blueprints for a Go + Vue application in the estate. This
template is their reference implementation; an application made from it specialises them. They moved
here from [`JorisJonkers-dev/grimoire`](https://github.com/JorisJonkers-dev/grimoire), whose own
`ARCHITECTURE.md` still wins over them for grimoire.

| Blueprint | Used for |
|---|---|
| [go-api.md](go-api.md) | the Go module: hexagonal API, spec-first OpenAPI (ogen), sqlc, goose |
| [vue-ts-spa.md](vue-ts-spa.md) | `web/`: Vue 3 + TypeScript SPA with a generated, validated client |

Rails, Rust, .NET and Spring/Kotlin variants were evaluated for grimoire and not chosen
([grimoire's ADR-0001](https://github.com/JorisJonkers-dev/grimoire/blob/main/docs/adr/0001-go-backend-not-estate-kotlin.md)).
The estate's decision to build new services this way is
[workspace ADR-0016](https://github.com/JorisJonkers-dev/workspace/blob/main/docs/decisions/ADR-0016-go-vue-contract-first-standard.md).

## Where the template stops short of the blueprints

The template wires every gate a new repository needs on day one. These parts of the blueprints are
deliberately left for the application to add when it has code worth guarding with them:

| Blueprint section | Not in the template | Why |
|---|---|---|
| go-api.md §6.2, §15 | NilAway (custom-gcl), `exhaustruct` | a module plugin build and a linter that pays off only once the domain has invariants to guard |
| go-api.md §16 | gremlins mutation testing | a mutation gate needs a core with real logic to mutate |
| go-api.md §6.3, §16, §19.2 | `govulncheck`, `sqlc vet` | `golangci-lint`'s `gosec` and `sqlc generate` gate the sample today; `sqlc vet` needs rules or a live database to check anything, and `govulncheck` joins with the first dependency worth scanning |
| go-api.md §19.4 | request logging middleware, request ids | the binary logs its lifecycle and every 500's cause; add the middleware with the first service that needs to trace a request |
| go-api.md §19.2, vue-ts-spa.md §19.2 | actions pinned to SHAs, `pnpm audit` | the estate's workflows pin actions by tag and let Renovate and Dependabot move them; an audit gate fails on advisories no change in the repository caused |
| vue-ts-spa.md §14, §17 | bundle budgets | the sample's bundle has no size worth budgeting yet |
| go-api.md §19.1 | CORS, rate limiting | the SPA and API share one origin, and the sample API does not rate-limit; `openapi/.spectral.yaml` says which OWASP rules to turn back on |
| go-api.md §20, §21 | River jobs, the WebSocket hub | optional; add when needed |
| vue-ts-spa.md §2, §15 | `eslint-plugin-boundaries` | one feature slice has no boundary to cross yet |
| vue-ts-spa.md §5, §19.3 | a separate static artifact behind a CDN | the Go binary embeds the build and serves it from the same origin, so there is one image and no runtime config to inject |
| vue-ts-spa.md §6.1 | `exactOptionalPropertyTypes` | the generated client does not compile under it |
| vue-ts-spa.md §10 | Pinia | the sample has no client state; add Pinia with the first piece of it |
| vue-ts-spa.md §13, §16 | MSW | the unit suite stubs `fetch` through `configureApi`; add MSW when that grows unwieldy |
