# Vue + TypeScript SPA — Architecture & Build Blueprint (generic)

> A blueprint for a **modular, maintainable, extensible, verifiable, regeneratable, vibe-mutatable,
> contract-oriented** Vue 3 + TypeScript single-page app. **Vue 3.5 + TypeScript 5.9 + Vite 8.**
> Layered architecture, an **OpenAPI-generated typed client with runtime validation**, strict typing,
> server-state/client-state separation, full coverage + a11y gates, strict ESLint, and a hardened
> supply chain that directly answers the JS/TS ecosystem's security problem.

**This document is the source of truth for agents building and extending the app.** It is the client
counterpart to the backend blueprints (Go/Rust/.NET/Spring/Rails) and consumes the same OpenAPI
contract. Numbered restrictions are **gates**, not suggestions. Keep the structure.

---

## 0. The seven properties this architecture exists to guarantee

- **Modular** — layered (presentation → application → domain → infrastructure) with a feature-sliced
  layout; the dependency rule enforced by lint.
- **Maintainable** — strict TypeScript, no `any`, generated client code never hand-edited, one
  obvious place for each concern.
- **Extensible** — composables as the application seam; new behavior is a new composable/feature, not
  a rewrite. The "add a feature" recipe (§17) is the only motion.
- **Verifiable** — `vue-tsc` strict typing, runtime response validation (Zod), full coverage on the
  domain, and a11y gates make correctness a CI question even in the browser.
- **Regeneratable** — the entire API layer (types + SDK + Zod schemas + query hooks) is **generated
  from the backend's OpenAPI document**; you regenerate rather than hand-write, and a contract change
  surfaces as TypeScript errors at the call sites.
- **Vibe-mutatable** — *the gates are the safety net.* Strict types + a generated client + runtime
  validation + a fast test suite mean a human or AI agent can change UI and data flow and learn
  instantly and locally if a change is wrong — including when the backend contract shifts.
- **Contract-oriented** — the backend's OpenAPI 3.1 document is the single source of truth; the
  client is wholly derived from it, and runtime validation enforces it at the boundary.

**Why Vue + TS is well-suited, and the honest tradeoff.** Vue 3's `<script setup>` + TS gives
excellent typed ergonomics; the OpenAPI toolchain makes the whole network layer generated and typed
end-to-end with the backend. The honest tradeoff is the **JS/TS supply chain** — the worst of any
stack here (2025–26 saw self-propagating npm worms). This blueprint treats that as a first-class
problem and answers it with pnpm's modern security defaults and a verified, frozen, cooldown'd
dependency policy (§19.2); it's why the *backend* in this family is never TS/JS.

---

## 1. Goals & non-negotiables

| Requirement | Decision |
|---|---|
| Framework | **Vue 3.5** (stable), `<script setup>` SFCs, API-driven SPA. |
| Language | **TypeScript 5.9**, strictest practical tsconfig (§6); no `any`. |
| Build/tooling | **Vite 8** (Rolldown/Oxc), **Vitest 4**, **vue-tsc** (template typechecking = the type gate). |
| Package manager | **pnpm 11** on **Node 22+**, with security defaults ON and a frozen, verified lockfile (§19.2). |
| API client | **Generated from the backend OpenAPI 3.1 doc** via `@hey-api/openapi-ts` (+ Zod runtime validation) (§13). |
| Server vs client state | **TanStack Query (or Pinia Colada)** for server state; **Pinia** for client/UI state only — never mixed (§10). |
| Typing model | Branded types for IDs, discriminated unions + `never` exhaustiveness, runtime validation at the boundary (§6). |
| Error model | Typed results from the client layer → mapped to typed UI error states; backend RFC 9457 surfaced as typed problems (§10). |
| Test coverage | Domain/composables **100%** (Vitest v8); generated client excluded; **Playwright + axe** a11y gate (§16). |
| Lint/style | **ESLint 9 flat config** + typescript-eslint type-checked + eslint-plugin-vue + vuejs-accessibility + boundaries (§15). |
| Architecture | Layered + feature-sliced; the dependency rule enforced by **eslint-plugin-boundaries** (§2/§7). |
| Security | CSP/security headers, static build artifact, runtime config injection (no baked secrets) (§19). |
| Deployment | Static build artifact behind a CDN/edge; config injected at runtime (§19). |

---

## 2. The dependency rule

Layers depend **inward only**:

```
presentation (components, pages, router)
   → application (composables, TanStack Query hooks, Pinia stores)
      → domain (pure TS: types, value objects, business rules)
   → infrastructure (GENERATED OpenAPI client, Zod schemas) — implements the data ports
```

Domain is **pure TypeScript** — no Vue, no fetch, no generated code. Components never call the API
client directly; they go through composables. **Enforcement:** **eslint-plugin-boundaries** declares
the layers and **fails CI** when a component imports infrastructure directly, or the domain imports
Vue.

---

## 3. Feature slices (the frontend's bounded contexts)

Organize by feature, not by file type. Each feature owns its components, composables, domain types,
and query hooks, and exposes a small public surface (a barrel `index.ts`). Cross-feature use goes
through that surface only (a boundaries rule enforces it). Shared primitives live in a `shared/` layer.
This mirrors the backend's bounded contexts so the two sides stay legible together.

---

## 4. Tech stack (pinned; pre-1.0 deps pinned exactly)

```jsonc
// package.json (excerpt)
{
  "dependencies": {
    "vue": "3.5.x", "vue-router": "4.x", "pinia": "3.x",
    "@tanstack/vue-query": "5.x", "zod": "3.x"
  },
  "devDependencies": {
    "typescript": "5.9.x", "vite": "8.x", "vitest": "4.x", "vue-tsc": "2.x",
    "@hey-api/openapi-ts": "0.99.x",          // PIN EXACT — pre-1.0
    "eslint": "9.x", "typescript-eslint": "8.x", "eslint-plugin-vue": "10.x",
    "eslint-plugin-vuejs-accessibility": "2.x", "eslint-plugin-boundaries": "5.x",
    "@playwright/test": "1.x", "@axe-core/playwright": "4.x", "msw": "2.x"
  }
}
```

Pin pre-1.0 tools exactly; keep the dependency count low — every package is supply-chain surface
(§19.2).

---

## 5. Runtime — the browser + a static artifact

The deliverable is a **static build artifact** (HTML/CSS/JS) served from a CDN/edge; there is no Node
server in production. Runtime config (API base URL, feature flags) is injected at deploy time (an
`/config.js` or `window.__CONFIG__`), never baked into the bundle — so the same artifact promotes
across environments and no secret is ever compiled in.

---

## 6. Type system — strict, branded, validated at the edge

**6.1 Strictest practical tsconfig:** `strict`, `noUncheckedIndexedAccess`,
`exactOptionalPropertyTypes`, `noImplicitOverride`, `noFallthroughCasesInSwitch`,
`verbatimModuleSyntax`. No `any`; `unknown` at untyped edges, narrowed immediately.

**6.2 Branded types for IDs** (nominal typing in a structural language):
```ts
type Brand<T, B> = T & { readonly __brand: B };
type UserId = Brand<string, "UserId">;
type OrderId = Brand<string, "OrderId">;   // passing a UserId where OrderId is expected = type error
```

**6.3 Discriminated unions + `never` exhaustiveness** (the compiler-checked "one of N"):
```ts
type PaymentResult =
  | { kind: "captured"; reference: string }
  | { kind: "declined"; reason: DeclineReason }
  | { kind: "pendingReview" };
function describe(r: PaymentResult): string {
  switch (r.kind) {
    case "captured":      return `captured ${r.reference}`;
    case "declined":      return `declined ${r.reason}`;
    case "pendingReview": return "pending";
    default:              return assertNever(r);   // add a variant → compile error here
  }
}
```

**6.4 Runtime validation at the boundary.** TypeScript types vanish at runtime, so the generated
client validates every response against **Zod** schemas (§13) — the boundary where the backend's
contract is *proven*, not assumed. Domain value objects validate on construction.

---

## 7. Repository layout

```
app/
├── package.json  pnpm-lock.yaml  tsconfig.json  vite.config.ts  eslint.config.ts
├── openapi/openapi.yaml                # the backend contract (source of truth) — pulled in CI
├── src/
│   ├── shared/                         # design system, utils, branded-type helpers
│   ├── features/<feature>/{components, composables, domain, api}/   # feature slices (§3)
│   ├── infrastructure/api/             # GENERATED client (types + SDK + Zod) — never hand-edit
│   ├── app/                            # router, providers, runtime config
│   └── main.ts
└── tests/{unit (Vitest), e2e (Playwright + axe)}/
```

`eslint-plugin-boundaries` enforces the layer + feature surfaces (§2).

---

## 8. Data layer (the frontend's "persistence") — generated & validated

The infrastructure layer is **wholly generated** from the backend OpenAPI doc and is the only place
that talks to the network. Components/composables consume typed SDK functions and never see raw fetch.
No JSONB-style untyped blobs cross the boundary — the generated types + Zod schemas mirror the
backend's normalized, typed contract exactly. (See §13 for the generator config; §10 for how state is
managed on top.)

---

## 9. OpenAPI conformance — the same gates as the backend

The frontend consumes the same OpenAPI 3.1 contract and benefits from the same strictness gates
(**Redocly `recommended-strict`** + **vacuum** + **Spectral**/OWASP). In CI the frontend **pulls the
backend's committed spec**, regenerates the client, and **fails on drift** (`git diff --exit-code`) —
so a backend contract change that hasn't been regenerated, or a hand-edit to generated code, breaks the
build.

---

## 10. Application layer — composables; server vs client state, strictly separated

Composables are the application seam. **The cardinal rule: server state and client state never mix.**

- **Server state** (anything that lives on the backend) is owned by **TanStack Query** (or Pinia
  Colada): caching, revalidation, loading/error, mutations + invalidation. Wrap query options in
  `computed()` so reactive keys work.
- **Client/UI state** (modals, wizard steps, selections, theme) is owned by **Pinia** — and *only*
  that. Never cache server data in Pinia.

```ts
export function useResource(id: Ref<ResourceId>) {
  return useQuery(computed(() => ({
    queryKey: ["resource", id.value],
    queryFn: () => api.getResource({ path: { id: id.value } }),   // generated, Zod-validated
  })));
}
```

A mutation returns a typed result; the component maps it to a typed UI state. Backend **RFC 9457**
problems are surfaced as typed problem objects, not stringly-typed errors.

---

## 11. Design pattern catalogue

Ports & adapters (data ports implemented by the generated client); Composable (the use-case unit);
Value Object (branded type / validated class); Discriminated union (+ `never` exhaustiveness);
Server-state cache (TanStack Query); Client-state store (Pinia); Presentational vs container
components; Result/typed-error mapping; MSW request mocking for tests. The app root wires providers
(Query client, Pinia, router) once.

---

## 12. External integrations

Anything beyond the primary API (analytics, feature flags, third-party widgets) is an adapter behind a
typed port in `infrastructure/`, lazy-loaded, and mockable in tests — never called inline from
components. No third-party script gets unrestricted DOM/network access (CSP, §19).

---

## 13. API client generation (the heart of contract orientation)

Generate the entire API layer from the backend's OpenAPI 3.1 doc with **`@hey-api/openapi-ts`**,
plugins: **`@hey-api/typescript`** (types), **`@hey-api/sdk`** (`validator: true` → **runtime Zod
validation of responses**), **`zod`** (schemas), and **`@tanstack/vue-query`** (generated query
options). Generated code goes in `src/infrastructure/api/` and is **never hand-edited**. MSW mirrors
the same spec for tests. A contract change → regenerate → TypeScript errors light up exactly the call
sites that must change (the regeneratable + vibe-mutatable payoff). CI drift-checks the generated
output (§9).

---

## 14. Performance & efficiency

Route-level code splitting + lazy components; Vite manual chunks for vendor; TanStack Query caching +
`staleTime` to cut refetching; virtualized long lists; `<img loading="lazy">` + responsive images;
preconnect to the API origin; keyset-paginated lists (matching the backend). Track bundle budgets in
CI. (Vue 3.6 **Vapor Mode** is a future opt-in for hot paths once stable — adopt in a bounded way, not
wholesale.)

---

## 15. Linting & style — ESLint 9 flat config, type-checked

**ESLint 9 (flat config)** with **typescript-eslint** type-checked rules, **eslint-plugin-vue** (Vue
3 recommended), **eslint-plugin-vuejs-accessibility** (a11y), and **eslint-plugin-boundaries** (the
§2 dependency rule). No `any` (`@typescript-eslint/no-explicit-any` error); exhaustive-switch enforced;
Prettier (or ESLint stylistic) for format. Zero issues is a gate; a disable needs an inline reason.

---

## 16. Testing, typing & coverage gates

Blocking: **`vue-tsc --noEmit`** (types + templates), **Vitest** unit suite with coverage (**domain/
composables 100%**, generated client excluded), **Playwright** e2e with **axe** a11y assertions
(a11y is a gate, not a nicety), ESLint, and the OpenAPI drift gate (§9). Layers: domain/composable
unit tests (jsdom, MSW for the network, the bulk) → component tests (Vue Test Utils) → e2e (Playwright
+ axe). MSW is generated from the same spec, so tests exercise the real contract shape.

---

## 17. Conventions for agents + Definition of Done

**Recipe — add a feature:** confirm the backend spec has the endpoints; pull spec + regenerate the
client (drift-checked) → add domain types/value objects (pure TS) in the feature slice → a composable
wrapping a generated query/mutation (server state via TanStack Query) → components consuming the
composable (client state via Pinia only) → tests (domain 100%, component, e2e + axe) → boundaries +
`vue-tsc` green.

**DoD (all blocking):** `vue-tsc` clean (no `any`); domain/composables 100% coverage; Playwright + axe
pass; ESLint clean (incl. boundaries + a11y); generated client regenerated + drift-checked (no
hand-edits); server state in TanStack Query / client state in Pinia (not mixed); responses Zod-
validated at the boundary; no secrets in the bundle; bundle budgets met.

---

## 18. Suggested milestones

M0 skeleton (Vue 3.5 + TS strict, Vite/Vitest/vue-tsc, hey-api client gen + Zod + MSW, ESLint flat +
boundaries + a11y, Playwright + axe, pnpm security defaults + frozen lockfile — every §16 gate green on
an empty app) → M1 first feature read path → M2 core domain logic → M3 mutating flows (typed errors) →
M4 full feature set → M5 hardening (CSP, bundle budgets, perf).

---

## 19. Modern standards

**19.1 Security (runtime).** Strict **CSP** (nonce/hash; no `unsafe-inline`), HSTS, `X-Content-Type-
Options`, frame-deny, Referrer-Policy — served as headers from the edge. No secret in the bundle;
runtime config injected (§5). Sanitize any HTML; third-party scripts are CSP-restricted.

**19.2 Supply chain (the answer to the JS problem).** **pnpm 11 security defaults ON**:
`minimumReleaseAge` (raise to ~1 week so freshly published malicious versions can't be pulled),
`blockExoticSubdeps`, build scripts blocked except an `allowBuilds` allowlist, `no-downgrade` trust
policy; **`--frozen-lockfile` in CI**; commit `pnpm-lock.yaml`. Add **Socket**/Snyk and `pnpm audit`
as CI gates; Renovate with a cooldown; pin GitHub Actions to SHAs; pin pre-1.0 deps exactly. This is
the deliberate, first-class mitigation of the 2025–26 npm-worm class of attacks — and the reason the
backend is never TS/JS.

**19.3 Deployment.** A static, content-hashed build artifact behind a CDN/edge; immutable assets +
long cache; `index.html` short-cached; SPA fallback routing; runtime config injected at deploy. The
same artifact promotes across environments.

**19.4 Observability.** Client error + performance reporting (e.g. Sentry/Faro) with release + trace
ids that correlate to backend traces; Web Vitals tracked; noisy console telemetry filtered.

**19.5 API conventions (client side).** Consume the versioned API; handle keyset pagination; render
**RFC 9457** problems as typed UI states; retries/backoff via TanStack Query, not hand-rolled.

**19.6 DX.** `pnpm dev` (Vite HMR) against MSW or a real backend; pre-commit runs `vue-tsc` + ESLint +
fast unit tests on staged files; CI mirrors §16 in parallel jobs.

---

### Verified facts (2026-06)
**Vue 3.5** is the stable line; **Vue 3.6 Vapor Mode** is feature-complete but not yet the default —
use 3.5, adopt Vapor only as a bounded opt-in. **TypeScript 5.9**, **Vite 8** (Rolldown/Oxc),
**Vitest 4**, **vue-tsc** (template typecheck = the type gate). **Node 22+**, **pnpm 11** with
supply-chain defaults ON (`minimumReleaseAge`, `blockExoticSubdeps`, blocked build scripts,
`no-downgrade`; `--frozen-lockfile` in CI) — the deliberate mitigation of the 2025–26 npm worm class.
API client via **@hey-api/openapi-ts** (~0.99.x, pin exact) with **@hey-api/typescript** +
**@hey-api/sdk** (`validator: true` → runtime **Zod** validation) + **@tanstack/vue-query**; **MSW**
for mocks. State: **Pinia** = client state only; **TanStack Query**/**Pinia Colada** = server state
(never mixed). Lint: **ESLint 9 flat config** + **typescript-eslint 8** type-checked +
**eslint-plugin-vue 10** + **vuejs-accessibility** + **eslint-plugin-boundaries**. Coverage: **Vitest
v8** (domain 100%, generated client excluded); **Playwright** + **axe** (a11y gate). Conformance via
**Redocly**, **vacuum**, **Spectral** + OWASP (shared with the backend contract).
