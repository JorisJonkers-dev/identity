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

A contract-first Go + Vue application template. `task check` and `task e2e` are everything CI runs;
`task` lists the targets. The toolchain is pinned in `mise.toml` (`mise install`).

- **The contract comes first.** Change `openapi/v1/openapi.yaml`, then `task gen`. Never hand-edit
  `internal/platform/oas/`, `internal/platform/pg/queries/` or `web/src/infrastructure/api/`.
- **The architecture is in `docs/blueprints/`.** Its numbered rules are gates; the README there
  says which ones the template has not wired yet.
- See `README.md` for how to turn a copy into an application.
