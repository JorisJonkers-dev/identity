---
status: accepted
---

# Anyone may sign up, and a new User can do nothing but edit their own Profile until an Admin assigns a Group

Signup is open and requires a confirmed email before the first sign-in. Until the email is confirmed no User exists: a pending sign-up holds the credentials, and confirming creates exactly that account, so nobody can sign up with someone else's address first and take the account over when its owner confirms. Mail never carries text the signer-up chose, because the address may not be theirs. A confirmed User in no Group is an Unassigned User: they can sign in and edit their Profile, and every Client refuses them. Admins find them in the portal's Unassigned queue, and identity raises an info alert through Alertmanager ([workspace ADR-0019](https://github.com/JorisJonkers-dev/workspace/blob/main/docs/decisions/ADR-0019-alertmanager-is-the-alert-hub.md)). Admins must hold a Second Factor (passkey or TOTP); for everyone else it is optional. The first Admin of a fresh install is made with `identity admin promote <email>`, run in the pod, which refuses when an Admin already exists unless forced.

## Considered Options

- **Invite only.** Smallest attack surface, but every new person needs an Admin to act first.
- **Second Factor for everyone.** Strongest, with more friction for family and friends than the access they hold warrants.
- **A bootstrap Admin email in configuration.** Easy, and it stays in configuration forever.
