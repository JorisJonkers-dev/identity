---
status: accepted
---

# A native app asks for a fresh two-factor proof with acr_values=mfa and max_age

A gated app asks the Gate for a fresh proof per action (ADR 0022). A native app could only rely on its Client's step-up window (ADR 0019), set by an Admin for the whole app. A native app now asks for one sign-in at a time with the standard OIDC parameters. It sends `acr_values=mfa` with `max_age=<seconds>` on `/oidc/authorize`, and identity requires the session's last two-factor proof to be no older than that:

- `max_age` is kept between 60 and 3600 seconds, matching the Gate's 1 to 60 minutes;
- without `max_age`, the proof may be at most an hour old;
- the stricter of this and the Client's step-up window applies.

A stale session goes to the same step-up page a Step-up Client uses, and a User with no two-factor is sent to set it up. Without `acr_values=mfa`, `max_age` changes nothing. The request keeps the window it asked for in `auth_requests.two_factor_within_seconds`.

## Considered Options

- **The Client-wide step-up window only.** No identity change, but a native `requireRecentTwoFactor(minutes)` could then only enforce the Admin's window, never the one an action needs.
- **`max_age` alone, read as the age of the two-factor proof.** One parameter, but it would change what `max_age` means to every OIDC library, which reads it as the age of the sign-in.
- **A parameter of identity's own.** Precise, and every language's OIDC library would need a custom option, where `acr_values` and `max_age` are already standard.

## Consequences

- A native sign-in kit's `requireRecentTwoFactor(minutes)` starts a sign-in with `acr_values=mfa&max_age=<minutes × 60>`, and reads the new proof from the access token's `two_factor_at`.
- identity does not yet state `acr` in its tokens. The app reads the proof's time from `two_factor_at`.
