---
status: accepted
---

# Access is Group-to-App-Role Assignments, roles add up, tokens carry only their own Client's roles, and Admins get no bypass

People are never given access directly: a User belongs to Groups, and an Assignment gives a Group one App Role on one Client. A User's roles on a Client are the union over their Groups, and identity never ranks roles; the app decides what a set means. A token or ID token for a Client carries only that Client's App Roles, never Group names, so renaming a Group cannot break an app. Admin is membership of the admins Group and lets a person manage Users, Groups and Assignments, but gives no access to any Client: a stolen Admin session does not open every app, unlike auth-api's `ROLE_ADMIN`. Groups and Assignments live in Postgres and are edited in the portal, even though Clients are GitOps, because adding a family member should not need a pull request.

## Considered Options

- **Per-user, per-app grants (today).** Simplest, but every new User or Client multiplies the edits.
- **Highest role wins.** Needs every Client to rank its roles, which breaks for roles that are not a ladder.
- **Send Group names to apps.** Lets an app key off groups, and couples every app to Group naming.
- **Admin bypass, or bypass after re-entering a second factor.** Convenient, and turns one Admin account into a key to everything.
- **Groups and Assignments as CRDs.** Reviewable, and a pull request per person added.
