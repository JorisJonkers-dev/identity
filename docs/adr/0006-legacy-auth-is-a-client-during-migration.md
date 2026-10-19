---
status: accepted
---

# Legacy Auth becomes a Client of identity, and Users are imported once

During migration, auth-api and auth-ui keep running as Legacy Auth, but they stop signing anyone in: their login page becomes "Sign in with jorisjonkers.dev", which makes Legacy Auth a Client of identity, and it keeps gating the hosts not yet moved. Users, bcrypt hashes, TOTP secrets and `SERVICE_*` grants are imported into identity once, so there is one user store from the first day. Imported bcrypt hashes are checked as they are and rehashed with argon2id on the next sign-in; each `SERVICE_*` grant becomes Group membership with that Client's `user` role, and today's ADMINs join the admins Group.

## Considered Options

- **Two user stores kept in sync.** No change to auth-api, but a password or second factor changed in one place drifts from the other.
- **Big bang.** One cutover window, with every app's sign-in changing in the same night.

## Consequences

auth.jorisjonkers.dev moves from the auth project to identity at the first cutover, so identity's project file cannot be published before it. Legacy Auth's `.jorisjonkers.dev` cookie stays until it gates no host, then auth-api and auth-ui are archived.

[0018](0018-users-and-machine-identities-hold-assignments-directly.md) changed one step: each `SERVICE_*` grant becomes a direct Assignment of that Client's `user` role, not Group membership.
