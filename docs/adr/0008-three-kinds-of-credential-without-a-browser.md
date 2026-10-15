---
status: accepted
---

# Without a browser, a credential is a Personal Token, a Machine Identity or a Connected App

auth-api's service tokens split into three kinds by who they act for. A **Personal Token** is minted by a User for one Client, lasts 30, 90 or 365 days at their choice, and never carries more than that User's current App Roles there. A **Machine Identity** acts for no person and holds its own Assignments; inside the cluster it exchanges its projected ServiceAccount token for an access token (RFC 8693), so there is no secret to store, and outside the cluster it uses a client secret. A **Connected App** is a tool that registered itself at runtime, such as an MCP client following the MCP authorization flow (OAuth 2.1 with PKCE, protected resource metadata, resource indicators); it acts for one consenting User on one Client, is listed in their Profile, and is not a Client.

## Considered Options

- **Personal tokens only, with bot Users for workloads.** One kind, and the User list fills with machines.
- **Tokens that never expire (today).** No rotation work, and a leaked token is valid forever.
- **No self-registration for MCP tools.** Simpler, and breaks the OAuth flow those tools expect.
- **Pre-register each MCP tool in deploy-kit.** Keeps one source of Clients, and needs a pull request per tool.
