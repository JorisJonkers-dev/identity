---
status: accepted
---

# A Second Factor counts only when the session proved it, and changing one needs both the password and that proof

A session records how it was proved: "pwd" by a password, "otp" once an authenticator app's code was given, and "hwk" once a passkey was. The admin pages need a session with "otp" or "hwk", not just an Admin who has a Second Factor somewhere, so a session from before the Admin enrolled, or one taken from them, does not open them. Adding an authenticator needs the password again, and a User who already has one must be in a session that proved it; otherwise whoever holds a password-only session could add their own app and so turn that session into a proved one. Removing an authenticator needs the password and a proved session, so a known password and an old session cannot take the Second Factor away. Adding the first authenticator ends every other session of the User's, as changing the password does, so no password-only session outlives it.

Wrong codes count against the User, not the sign-in: a User has one sign-in challenge, a new sign-in replaces it but keeps its spent attempts, and five wrong codes block codes for fifteen minutes. Otherwise anyone who knows the password could start sign-in after sign-in and guess codes without limit. A code's 30-second step is recorded when it is accepted, in one conditional update, so a code works once even when two sign-ins race for it. TOTP secrets are Vault transit ciphertext in the database (ADR 0013); identity asks Vault to open one only to check a code.

A passkey is the other kind of Second Factor and follows the same rules: adding one needs the password and, when the User has a Second Factor, a proved session; removing one needs both; a sign-in with one is proved "pwd" and "hwk"; a refused passkey answer counts as a wrong code. The WebAuthn ceremonies are go-webauthn's, as OIDC is zitadel/oidc's; identity keeps the public key and the library's record of it. A signature counter that did not move on means a second copy of the key signed, which the library only flags; identity refuses it, and the passkey stays refused. User verification is preferred, not required, because the password has already been checked.

## Considered Options

- **"Is an Admin with an authenticator" as the admin check.** Simpler, and lets a session from before enrolment, or a stolen one, manage access.
- **Attempts per challenge.** Simpler, and gives a password holder unlimited guesses across sign-ins.
- **Lock the account after wrong codes.** Stops guessing, and lets anyone who knows the password lock the owner out for good.
- **Enrolment without the password.** One step fewer, and lets a stolen session prove itself.
- **Passkeys as the only factor, without a password.** Faster to sign in, and a separate decision with its own recovery questions; passkeys stand in for the code for now.
- **Accept a passkey whose counter stood still.** Some authenticators never count, and an always-zero counter is still accepted; a counter that went backwards or stood still after counting is a copied key.
