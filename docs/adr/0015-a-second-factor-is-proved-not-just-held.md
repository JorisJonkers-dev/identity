---
status: accepted
---

# A Second Factor counts only when the session proved it, and changing one needs both the password and that proof

A session records how it was proved: "pwd" by a password, and "otp" once an authenticator app's code was given. The admin pages need a session with "otp", not just an Admin who has an authenticator somewhere, so a session from before the Admin enrolled, or one taken from them, does not open them. Adding an authenticator needs the password again, and a User who already has one must be in a session that proved it; otherwise whoever holds a password-only session could add their own app and so turn that session into a proved one. Removing an authenticator needs the password and a proved session, so a known password and an old session cannot take the Second Factor away. Adding the first authenticator ends every other session of the User's, as changing the password does, so no password-only session outlives it.

Wrong codes count against the User, not the sign-in: a User has one sign-in challenge, a new sign-in replaces it but keeps its spent attempts, and five wrong codes block codes for fifteen minutes. Otherwise anyone who knows the password could start sign-in after sign-in and guess codes without limit. A code's 30-second step is recorded when it is accepted, in one conditional update, so a code works once even when two sign-ins race for it. TOTP secrets are Vault transit ciphertext in the database (ADR 0013); identity asks Vault to open one only to check a code.

## Considered Options

- **"Is an Admin with an authenticator" as the admin check.** Simpler, and lets a session from before enrolment, or a stolen one, manage access.
- **Attempts per challenge.** Simpler, and gives a password holder unlimited guesses across sign-ins.
- **Lock the account after wrong codes.** Stops guessing, and lets anyone who knows the password lock the owner out for good.
- **Enrolment without the password.** One step fewer, and lets a stolen session prove itself.
