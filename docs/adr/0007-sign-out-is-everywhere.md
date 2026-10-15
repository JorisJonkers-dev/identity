---
status: accepted
---

# Signing out of any app signs out everywhere, and sessions last 30 days, 12 hours for Admins

A sign-out from any Client ends the Single sign-on session, sends OIDC back-channel logout to every native Client that supports it, and drops the person's Gate sessions; ID tokens carry `sid` so a logout ends exactly that session. This is what makes an Admin's "force sign-out" mean something. The Single sign-on session lasts 30 days and is extended on use; an Admin's lasts 12 hours.

## Considered Options

- **Sign out of that app only.** What many OIDC apps do by default, and surprising to anyone who expects one login to mean one logout.
- **Ask on every sign-out.** Precise, and one more page nobody reads.
- **7 days, or browser-session length.** Safer, with weekly password entry for people who use these apps casually.
