---
status: accepted
---

# Step-up is an Admin setting in identity, not part of the Client's declaration

Everything else about a Client comes from deploy-kit's `client` block, and nothing in identity's database describes a Client. Step-up is the exception. An Admin turns it on for a Client on that app's page and picks how fresh the two-factor proof must be: 5, 15 or 60 minutes. identity stores that setting.

Signing in to such a Client asks for a two-factor method whenever the session's last proof is older than the window, even inside a live Single sign-on session. A gated host's sign-in lasts only that window, and then the Gate sends the User back for a code. No Personal Token or Connected App can be made for a Step-up Client, because a bearer credential never proves anything fresh.

## Considered Options

- **Declared in the Client's `client` block.** This matches every other Client setting and is reviewed in Git. It needs a deploy-kit spec change and a release before an Admin can react. Step-up is a judgement about risk that the Admin makes, not a property of the app.
- **Per App Role.** This is finer, and it would make one Client behave differently for different holders. Nothing asked for it yet.

## Consequences

- identity's database now holds one piece of Client policy. A Removed Client keeps its setting, so it comes back the same.
- Turning step-up on revokes the Client's existing Personal Tokens and Connected Apps, because they could never satisfy it. The app's page says how many before the Admin confirms, and each revocation is an Audit Event.
- A User with no two-factor cannot open a Step-up Client. They are sent to set two-factor up.
