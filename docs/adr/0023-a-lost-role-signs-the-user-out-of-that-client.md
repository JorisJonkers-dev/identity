---
status: accepted
---

# A role taken away signs the User out of that Client by back-channel logout

A native app learns a User's App Roles only when the User signs in. It gets no refresh tokens (ADR 0014), userinfo carries no roles, and a public PKCE Client cannot introspect. So a role an Admin takes away kept working there until the app's own session ended.

Now, when a User loses an App Role on a Client, identity sends that Client a logout token for each of the User's sessions that signed into it (the same token as ADR 0007). That covers:

- an Admin taking a direct role away;
- a Group losing the role;
- the User leaving a Group, or the Group being deleted;
- a Group starting to require two-factor the User does not have.

identity reads the User's roles per Client before and after the change. A Client where a role is missing afterwards gets the token, even when the User still holds other roles there. A User who still holds the role another way is not signed out. The Single sign-on session stays, so the app's next sign-in comes straight back with the roles the User holds now.

## Considered Options

- **userinfo carries the Client's roles, and the app polls it.** No restart of the app's session, but zitadel/oidc hands userinfo only the token id and subject, not the Client. A removal would also land only when the app next polls.
- **The app runs a silent `prompt=none` sign-in every minute.** No identity change, but it costs a browser redirect each minute and does nothing for an app's API calls.

## Consequences

- The Principal contract's flow suite proves it as "a role taken away stops working", and every sign-in kit must end its session on a logout token.
- A Client without a `backChannelLogoutUri` still learns of the change only at its next sign-in.
- Every role removal reads each affected User's roles twice. For a Group, that is once per member.
- Gated apps are untouched: the Gate checks roles on every uncached request, and caches for at most 30 seconds.
