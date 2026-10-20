---
status: accepted
---

# Connected Apps get opaque access tokens and rotating refresh tokens, and only on MCP resources

A tool such as an MCP client registers itself at `/oidc/register` (RFC 7591) without asking anyone. Registration gives it nothing: it gets a token only after a signed-in User consents, on identity's consent page, to that tool acting for them on one Client. The Client is the one the tool names as its `resource` (RFC 8707). A registration nobody consents to within a day is pruned.

Only a Client an Admin marked as an **MCP resource** takes Connected Apps. A Step-up Client never does (ADR 0019).

A Connected App's access token is opaque, lasts an hour, and is kept in identity's database. The Gate and introspection check it there, with the App Roles the User holds on the Client at that moment. Its refresh token lasts 30 days and is kept only as a hash. Every refresh spends it and issues a new one. A refresh token spent a second time means a copy leaked, so the Connected App is disconnected and the event is audited.

Every other client keeps the 15-minute JWTs of ADR 0014 and gets no refresh tokens.

## Considered Options

- **JWT access tokens, as every other client gets.** A Client checks them without calling identity, but disconnecting a tool then takes up to 15 minutes to reach every Client. A person who disconnects a tool expects it to stop at once.
- **No refresh tokens, as ADR 0014 chose.** MCP clients run unattended for days. Without refresh tokens they would send the person back through consent every hour.
- **Registration by an Admin, or a deploy-kit pull request per tool.** This closes registration to strangers, but the MCP authorization flow expects to register itself. Consent already guards what a registration can do.
- **Any Client takes Connected Apps.** Simpler, but an Admin could not keep a sensitive app out of reach of tools.

## Consequences

- identity stores and prunes Connected App tokens. Disconnecting a tool, deleting its User, or turning step-up on for its Client drops its tokens and its cached Gate checks at once.
- A tool calls the Gate or introspection on every request, the same as a Personal Token (ADR 0017).
- `/.well-known/oauth-authorization-server` names the registration endpoint. `/.well-known/oauth-protected-resource/<host>` is each MCP resource's metadata, and the Gate's 401 on that host points to it.
