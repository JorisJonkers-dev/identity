# Identity

The estate's single sign-in: who a person is, which apps they may use, and in
what role. Every app signs people in here instead of trusting a shared cookie.

## Language

### People

**User**:
A person who has registered with the Identity Provider. A new User can sign in
and edit their own Profile, and has no access to any app until an admin gives it.
_Avoid_: Account, member, principal

**Profile**:
The details a User may edit themselves: name, email, password, two-factor methods.
_Avoid_: Account settings

**Unassigned User**:
A User who holds no Assignment, through a Group or directly. Admins see them in
a queue until they are placed.
_Avoid_: Pending user, guest

**Admin**:
A User in the admins Group, allowed to manage Users, Groups and access. Must
have two-factor. Being an Admin gives no access to any Client by itself.
_Avoid_: Superuser, root, ROLE_ADMIN

**Two-factor**:
Proving a two-factor method on sign-in in addition to the password. A User
"has two-factor" when they hold at least one two-factor method.
_Avoid_: Second Factor, MFA, 2FA

**Two-factor method**:
One way a User proves two-factor: a passkey or an authenticator app.
_Avoid_: MFA device, 2FA token, second factor

**Group**:
A named set of Users. Access is usually given to Groups; a User may also hold
an Assignment directly.
_Avoid_: Team, role

**Two-factor Group**:
A Group that requires two-factor. A member without two-factor holds none of
its Assignments until they set it up. The admins Group is always one.
_Avoid_: Secure group, MFA group

### Sign-in

**Identity Provider**:
The one service at `auth.jorisjonkers.dev` that signs Users in and vouches for
them to apps.
_Avoid_: Auth server, SSO server, auth-api

**Issuer**:
The fixed address, `https://auth.jorisjonkers.dev`, that every app trusts as
the Identity Provider.
_Avoid_: Auth URL, IdP URL

**Single sign-on session**:
The Identity Provider's own record that a User has signed in, which lets a
second app sign them in without a password.
_Avoid_: Session cookie, shared session

**Device**:
What the Security pages call one of a User's Single sign-on sessions: one
signed-in browser, which the User can sign out on its own.
_Avoid_: Trusted device, remembered browser

**Step-up**:
A fresh two-factor proof a Client demands before it opens, even inside a live
Single sign-on session. An Admin turns it on per Client and picks how fresh the
proof must be. No Personal Token or Connected App can be made for such a Client.

**Password reset**:
Choosing a new password through a mailed link, after proving two-factor if the
User has it. Every Single sign-on session of that User ends.
_Avoid_: Recovery, password recovery
_Avoid_: Re-auth, sudo mode, elevation

**Client**:
Anything that signs Users in through the Identity Provider: one per deploy-kit
Application that declares a `client` block. Users see a Client as an app in the
Launcher.
_Avoid_: Application (deploy-kit's release unit), service, relying party

**Sign-in Mode**:
How a Client signs Users in: `native` (the app speaks OIDC itself), `gated`
(the Gate signs in on the app's behalf) or `token` (no browser, bearer tokens
only).
_Avoid_: Auth mode, access mode

**Gate**:
The part of the Identity Provider that signs Users in for a Client whose app
cannot speak OIDC, and keeps a separate sign-in per host.
_Avoid_: Forward-auth, proxy, middleware

**Principal**:
Who is calling an application, as identity states it on every channel: the
User or Machine Identity, its App Roles on that Client, how it signed in, the
tool acting for it, and whether its session proved two-factor.
_Avoid_: Identity, user context, claims

### Access

**App Role**:
A role a Client declares for its own Users, such as `viewer` or `editor`.
Every Client has at least the role `user`.
_Avoid_: Permission, scope, SERVICE_*

**Assignment**:
A Group, a User or a Machine Identity holding one App Role on one Client. A
User holds every App Role assigned to them directly or to any of their Groups;
roles combine, they never override.
_Avoid_: Grant (deploy-kit's word for Secret Store access), permission, binding

**Launcher**:
The list of Clients a User holds any App Role on, shown at the Identity Provider.
_Avoid_: Home portal, my apps, dashboard

**Removed Client**:
A Client whose deploy-kit Application no longer exists. Its Assignments and
tokens are kept but give no access until the Client returns or an Admin purges it.
_Avoid_: Deleted client, orphan

### Credentials without a browser

**Personal Token**:
A bearer credential a User mints for one Client. It carries at most that User's
own App Roles on that Client and ends when the User does.
_Avoid_: Service token, API key, PAT

**Connected App**:
A tool that registered itself at runtime, such as an MCP client, and acts for
one consenting User on one Client, never beyond that User's App Roles there.
It is not a Client.
_Avoid_: Dynamic client, third-party app, integration

**MCP resource**:
A Client an Admin marked as taking Connected Apps. A tool names it as the
resource it wants to act on.
_Avoid_: MCP server, connected Client

**Machine Identity**:
A non-person that signs in to Clients with its own credentials and its own
Assignments, such as a workload calling another workload. It holds Assignments
directly and is never in a Group.
_Avoid_: Service account, bot user, service token

### Oversight

**Audit Event**:
A record that someone signed in, failed to, minted or revoked a token, or
changed a User, Group or Assignment.
_Avoid_: Log entry, activity

### Migration

**Legacy Auth**:
The old auth-api and auth-ui while they run alongside the Identity Provider.
It signs no one in itself: it is a Client that keeps gating the hosts not yet
moved.
_Avoid_: Old auth, auth-api (when meaning the role it plays during migration)
