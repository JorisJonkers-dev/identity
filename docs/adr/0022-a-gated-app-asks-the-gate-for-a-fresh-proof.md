---
status: accepted
---

# A gated app asks the Gate for a fresh two-factor proof, per action

Step-up was a Client setting only (ADR 0019): every request to the Client needs a proof inside the window. An app that wants a fresh proof for one dangerous action, and not for the rest, now sends the browser to `/.auth/step-up?minutes=<n>&rd=<path>` on its own host. The Gate sets the host's state and starts a sign-in at the Issuer asking for a proof no older than `n` minutes, from 1 to 60. The stricter of that and the Client's own window applies. A session with a fresh enough proof comes straight back. A staler one proves two-factor again on the step-up page first, and a User with no two-factor is sent to set it up. The new host sign-in drops every cached check of that Single sign-on session, so the next Principal states the new `twoFactorAt`.

## Considered Options

- **The app links to `/signin/step-up` itself.** That page returns only to paths on the Issuer, and the Gate's cache would keep stating the old proof for up to 30 seconds.
- **Make the whole Client a Step-up Client.** This works today, and asks for a proof on every page, not only the action that needs it.

## Consequences

- The adapters' `requireRecentTwoFactor(minutes)` answers 403 with a redirect to this path. The contract's `mapping.json` names the path.
- A request that came by Personal Token, tool or machine cannot step up, because no browser carries it. The adapters answer it 403 without a redirect.
- Every host sign-in now drops its session's cached checks on every host, which costs one check from the database on each.
