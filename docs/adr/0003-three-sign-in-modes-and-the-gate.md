---
status: accepted
---

# Every Client signs in one of three ways, and the Gate signs in per host for apps without OIDC

A Client's Sign-in Mode is `native` (the app runs the code flow itself), `gated` (the Gate runs it on the app's behalf) or `token` (bearer tokens only). The Gate replaces forward-auth with a shared cookie: on a gated host with no sign-in it serves a page with one sign-in button, receives the code on the host's own derived `/.auth/callback` path, checks the person's App Roles, and sets a `__Host-` cookie that only that host receives. Every app shows a sign-in button rather than redirecting on its own; after the first sign-in, the Single sign-on session makes the next app's button silent.

## Considered Options

- **Keep forward-auth with the `.jorisjonkers.dev` cookie.** Nothing to build, but every app receives a credential that opens every other app.
- **One oauth2-proxy deployment per gated app.** Proven software, but one more Deployment per app and access checks outside the place that holds Assignments.
- **Redirect to the login page without a button.** Matches today, but signing in stops being a visible choice.

## Consequences

The callback must come back to each gated host, so deploy-kit derives a `/.auth/` route on every gated host and refuses an app route under it. Apps whose TV or phone clients cannot follow a browser sign-in (Jellyfin) fit none of the three modes yet.
