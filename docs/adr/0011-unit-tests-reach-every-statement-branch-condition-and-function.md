---
status: accepted
---

# Unit tests reach every statement, branch, condition and function; integration tests reach every function

Tests come in three buckets. **Unit** tests need no Docker and run with a plain `go test` and `vitest`. **Integration** tests carry the `integration` build tag and run against Postgres and liquibase-runner in testcontainers. **End-to-end** tests are Playwright against the built binary. `task test:unit` holds every non-generated Go package that can run without a database, and every web source file but `main.ts`, to 100% of statements, branches and functions; Go also to 100% of conditions. Go measures only statements, so `scripts/coverage.sh` takes branches and conditions from gobco (each operand of `&&` and `||` both ways) and functions from the coverage profile. `task test:integration` runs both Go buckets together and holds every non-generated function to at least one test, except `main()`, which only the end-to-end suite starts. Totals land in `coverage/coverage-totals.json`.

The model is the one ESA-Blueshell/website's imaging service uses. It replaces the template's gate of 80% Go statements overall and 100% on `domain` and `app`.

## Considered Options

- **The grimoire bar**: 98% statements overall, 100% on the core, gremlins on the core. Mutation testing is a stronger signal per line, but it covers only the core, and statements say nothing about the branch a test never takes.
- **Statements only, at 100%.** Cheap, and blind to the `else` nobody wrote a test for.
- **One merged gate over unit and integration.** Simpler, but a unit suite that silently starts needing Docker would still pass.

## Consequences

- A decision gobco cannot reach is a design smell to fix, not a gap to exclude: an unreachable error branch is removed, and a panic on impossible input moves into a helper a test can drive.
- gobco copies the whole module before instrumenting it, so the script runs it from a copy without `node_modules`.
- The unit run takes minutes, not seconds, because gobco runs each package's tests twice.
