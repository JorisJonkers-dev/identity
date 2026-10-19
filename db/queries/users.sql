-- name: CreateUser :exec
INSERT INTO users (id, email, name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: UserByEmail :one
SELECT id, email, name, password_hash, created_at FROM users WHERE email = $1;

-- name: UserByID :one
SELECT id, email, name, password_hash, created_at FROM users WHERE id = $1;

-- name: UpdateUserName :exec
UPDATE users SET name = $2 WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: CreateSignup :exec
INSERT INTO signups (token_hash, email, name, password_hash, expires_at) VALUES ($1, $2, $3, $4, $5);

-- name: SignupByHash :one
SELECT email, name, password_hash, expires_at FROM signups WHERE token_hash = $1;

-- name: DeleteSignup :exec
DELETE FROM signups WHERE token_hash = $1;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, id, user_id, created_at, last_seen_at, expires_at, methods, two_factor_at, device) VALUES ($1, $2, $3, $4, $4, $5, $6, $7, $8);

-- name: SessionByHash :one
SELECT token_hash, id, user_id, created_at, last_seen_at, expires_at, methods, two_factor_at FROM sessions WHERE token_hash = $1;

-- name: SessionsOfUser :many
-- A User's live sessions, the most recently seen first: their Devices.
SELECT token_hash, id, device, created_at, last_seen_at FROM sessions
WHERE user_id = $1 AND expires_at > $2 ORDER BY last_seen_at DESC, id;

-- name: DeleteSessionOfUser :execrows
DELETE FROM sessions WHERE id = $1 AND user_id = $2;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = $2, expires_at = $3 WHERE token_hash = $1;

-- name: DeleteOtherSessions :many
-- Ends every session of a User but the one keep names (or every one, when keep is null), and says
-- which ended, so exactly those are announced (docs/adr/0007).
DELETE FROM sessions WHERE user_id = $1 AND token_hash IS DISTINCT FROM sqlc.narg(keep)::bytea RETURNING id;

-- name: ListUsers :many
SELECT id, email, name, password_hash, created_at FROM users ORDER BY lower(name), email;

-- name: SetSessionMethods :exec
-- A session that has just proved a Second Factor says so from then on, and when.
UPDATE sessions SET methods = $2, two_factor_at = $3 WHERE token_hash = $1;

-- name: UpsertChallenge :exec
-- A User has one challenge. A new sign-in replaces its token and expiry; its wrong codes still
-- count until the window that began with the first of them has passed.
INSERT INTO sign_in_challenges (token_hash, user_id, expires_at, window_started_at, session_hash) VALUES ($1, $2, $3, $4, sqlc.narg(session_hash))
ON CONFLICT (user_id) DO UPDATE SET
  token_hash = EXCLUDED.token_hash,
  session_hash = EXCLUDED.session_hash,
  passkey_ceremony = NULL,
  expires_at = EXCLUDED.expires_at,
  attempts = CASE WHEN sign_in_challenges.window_started_at <= sqlc.arg(window_since) THEN 0 ELSE sign_in_challenges.attempts END,
  window_started_at = CASE WHEN sign_in_challenges.window_started_at <= sqlc.arg(window_since) THEN EXCLUDED.window_started_at ELSE sign_in_challenges.window_started_at END;

-- name: SpendChallengeAttempt :one
-- Counts one code against a sign-in; past the last attempt nothing comes back.
UPDATE sign_in_challenges SET attempts = attempts + 1
WHERE token_hash = $1 AND attempts < $2 AND expires_at > $3
RETURNING user_id;

-- name: StepUpChallenge :one
-- The step-up challenge that proves this session again, if it has one.
SELECT token_hash FROM sign_in_challenges WHERE session_hash = $1;

-- name: DeleteChallenge :exec
DELETE FROM sign_in_challenges WHERE token_hash = $1;

-- name: TOTPFactorsOfUser :many
SELECT id, user_id, label, sealed_secret, last_step, created_at, last_used_at FROM totp_factors
WHERE user_id = $1 ORDER BY created_at, id;

-- name: CreateTOTPFactor :exec
INSERT INTO totp_factors (id, user_id, label, sealed_secret, last_step, created_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: UseTOTPStep :execrows
-- Accepts a code's step once: a replayed or older code changes nothing.
UPDATE totp_factors SET last_step = $2, last_used_at = $3 WHERE id = $1 AND last_step < $2;

-- name: DeleteTOTPFactor :execrows
DELETE FROM totp_factors WHERE id = $1 AND user_id = $2;

-- name: UpsertTOTPEnrolment :exec
INSERT INTO totp_enrolments (user_id, sealed_secret, expires_at) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET sealed_secret = EXCLUDED.sealed_secret, expires_at = EXCLUDED.expires_at;

-- name: TOTPEnrolment :one
SELECT sealed_secret, expires_at FROM totp_enrolments WHERE user_id = $1;

-- name: DeleteTOTPEnrolment :exec
DELETE FROM totp_enrolments WHERE user_id = $1;

-- name: PasskeysOfUser :many
SELECT id, user_id, label, credential_id, credential, created_at, last_used_at FROM passkeys
WHERE user_id = $1 ORDER BY created_at, id;

-- name: CreatePasskey :exec
INSERT INTO passkeys (id, user_id, label, credential_id, credential, created_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: UsePasskey :execrows
-- Keeps the signature counter and flags WebAuthn returned with the assertion, only if the record is
-- still the one the assertion was checked against: of two sign-ins racing with one key, one wins.
UPDATE passkeys SET credential = $3, last_used_at = $4
WHERE user_id = $1 AND credential_id = $2 AND credential = sqlc.arg(previous)::jsonb;

-- name: DeletePasskey :execrows
DELETE FROM passkeys WHERE id = $1 AND user_id = $2;

-- name: UpsertPasskeyEnrolment :exec
INSERT INTO passkey_enrolments (user_id, ceremony, expires_at) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET ceremony = EXCLUDED.ceremony, expires_at = EXCLUDED.expires_at;

-- name: PasskeyEnrolment :one
SELECT ceremony, expires_at FROM passkey_enrolments WHERE user_id = $1;

-- name: DeletePasskeyEnrolment :exec
DELETE FROM passkey_enrolments WHERE user_id = $1;

-- name: SetChallengeCeremony :execrows
-- The WebAuthn ceremony a sign-in started; only an unexpired challenge takes one.
UPDATE sign_in_challenges SET passkey_ceremony = $2 WHERE token_hash = $1 AND expires_at > $3;

-- name: ChallengeCeremony :one
SELECT user_id, passkey_ceremony FROM sign_in_challenges WHERE token_hash = $1 AND expires_at > $2;

-- name: TakeChallengeCeremony :one
-- Takes the ceremony a sign-in started in one statement: a second request waits on the row lock,
-- then finds the ceremony gone, so only one request can use it.
WITH taken AS (
  SELECT t.token_hash AS taken_hash, t.user_id, t.passkey_ceremony FROM sign_in_challenges t
  WHERE t.token_hash = sqlc.arg(token_hash) AND t.expires_at > sqlc.arg(now) AND t.passkey_ceremony IS NOT NULL
  FOR UPDATE
)
UPDATE sign_in_challenges c SET passkey_ceremony = NULL
FROM taken WHERE c.token_hash = taken.taken_hash AND c.passkey_ceremony IS NOT NULL
RETURNING taken.user_id, taken.passkey_ceremony;

-- name: SessionByID :one
SELECT token_hash, id, user_id, created_at, last_seen_at, expires_at, methods, two_factor_at FROM sessions WHERE id = $1;

-- name: CountActiveSessions :one
SELECT count(*) AS sessions, count(DISTINCT user_id) AS users FROM sessions WHERE expires_at > $1;

-- name: SaveEmailChange :exec
-- A User has one pending email change; asking again replaces it.
INSERT INTO email_changes (user_id, email, token_hash, expires_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE SET email = EXCLUDED.email, token_hash = EXCLUDED.token_hash, expires_at = EXCLUDED.expires_at;

-- name: EmailChangeOfUser :one
SELECT user_id, email, token_hash, expires_at FROM email_changes WHERE user_id = $1;

-- name: TakeEmailChange :one
-- Following the link spends it, whatever happens next.
DELETE FROM email_changes WHERE token_hash = $1 RETURNING user_id, email, token_hash, expires_at;

-- name: DeleteEmailChange :exec
DELETE FROM email_changes WHERE user_id = $1;

-- name: UpdateEmail :exec
UPDATE users SET email = $2 WHERE id = $1;

-- name: LockAdminsGroup :one
-- Held until the transaction ends, as removing an Admin holds it, so two deletions cannot both see
-- the other Admin.
SELECT id FROM groups WHERE admins FOR UPDATE;

-- name: IsOnlyAdmin :one
SELECT (EXISTS (SELECT 1 FROM memberships m WHERE m.group_id = sqlc.arg(admins)::uuid AND m.user_id = sqlc.arg(user_id)::uuid)
  AND (SELECT count(*) FROM memberships c WHERE c.group_id = sqlc.arg(admins)::uuid) = 1)::boolean AS only;

-- name: DeleteSessionsOfUser :many
DELETE FROM sessions WHERE user_id = $1 RETURNING id;

-- name: DeletePersonalTokensOfUser :many
DELETE FROM personal_tokens WHERE user_id = $1 RETURNING id;

-- name: DeleteUser :exec
-- Memberships, direct Assignments, factors and pending changes go with the User; Audit Events,
-- which keep only ids, stay.
DELETE FROM users WHERE id = $1;

-- name: SavePasswordReset :exec
-- A User has one reset at a time; asking again replaces it.
INSERT INTO password_resets (user_id, token_hash, expires_at) VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO UPDATE SET token_hash = EXCLUDED.token_hash, expires_at = EXCLUDED.expires_at, proved_at = NULL;

-- name: PasswordResetByHash :one
SELECT user_id, token_hash, expires_at, proved_at FROM password_resets WHERE token_hash = $1;

-- name: ProvePasswordReset :exec
UPDATE password_resets SET proved_at = $2 WHERE user_id = $1;

-- name: TakePasswordReset :execrows
-- Finishing a reset spends its link; of two racing, one gets it.
DELETE FROM password_resets WHERE token_hash = $1;

-- name: ListUsersWithTwoFactor :many
-- The Users who added an authenticator app or a passkey.
SELECT user_id FROM totp_factors UNION SELECT user_id FROM passkeys;
