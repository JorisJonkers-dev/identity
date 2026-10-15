# Agent contract

The estate-wide conventions live in one place and are **not duplicated here**:

**https://github.com/JorisJonkers-dev/workspace/blob/main/CLAUDE.md**

Read it before doing anything non-trivial in this repository. It covers the
things that most often go wrong, including:

- **Pull request labels.** The estate uses a prefixed taxonomy - `type:`,
  `area:`, `component:`, `priority:`, `status:`. Plain `bug` / `enhancement` /
  `documentation` do **not** exist, and `gh pr create` fails with
  `'bug' not found`. Run `gh label list --repo <owner>/<repo>` once before
  passing `--label`.
- **Verify the value, not the command.** An exit code, a `Ready` condition or
  an accepted object is not evidence that a consumer sees what you intended.
- Traps around workflow runs, `zsh` word-splitting, and detached submodule
  HEADs.

Duplicating that content into every repository guarantees the copies drift, so
this file stays a pointer. Add repo-specific guidance below.

## This repository

The estate's Identity Provider: one OIDC sign-in for every app, at the Issuer
`https://auth.jorisjonkers.dev`. It replaces auth-api and auth-ui
([workspace ADR-0020](https://github.com/JorisJonkers-dev/workspace/blob/main/docs/decisions/ADR-0020-identity-replaces-auth-api.md)).
Contract-first Go + Vue from `template-go-vue`. `task check` and `task e2e` are everything CI runs;
`task` lists the targets. The toolchain is pinned in `mise.toml` (`mise install`).

- **Use the words in `CONTEXT.md`.** Client, not Application (deploy-kit's release unit);
  Assignment, not Grant (deploy-kit's Secret Store access); User, not account.
- **Decisions are in `docs/adr/`.** Read them before changing sign-in, access, sessions or tokens.
- **Never implement OIDC protocol code here.** It comes from `zitadel/oidc`; identity implements its
  storage ([0001](docs/adr/0001-build-on-zitadel-oidc.md)).
- **Clients are read-only here.** They come from deploy-kit `Client` objects; nothing in this
  repository creates one ([0002](docs/adr/0002-clients-come-only-from-deploy-kit.md)).
- **The contract comes first.** Change `openapi/v1/openapi.yaml`, then `task gen`. Never hand-edit
  `internal/platform/oas/`, `internal/platform/pg/queries/` or `web/src/infrastructure/api/`.
- **The architecture is in `docs/blueprints/`.** Its numbered rules are gates; the README there
  says which ones the template has not wired yet.
- **The schema moves only through `db/changelog/`.** liquibase-runner applies it from
  `Dockerfile.migration`; the binary never migrates. Every changeset must roll back
  ([0010](docs/adr/0010-migrations-run-through-liquibase-runner.md)). After changing it, `task gen`.
- **CI runs only what a diff reaches.** `.github/buckets.yml` maps paths to jobs; a new file
  type or top-level file needs a bucket (or none, to run everything) and a fixture, or the
  `changes` job's self-test fails.
- **Still template state.** The sample `notes` context is the template's; it is replaced as the
  first features land.
