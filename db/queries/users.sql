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
INSERT INTO sessions (token_hash, id, user_id, created_at, last_seen_at, expires_at, methods) VALUES ($1, $2, $3, $4, $4, $5, $6);

-- name: SessionByHash :one
SELECT token_hash, id, user_id, created_at, last_seen_at, expires_at, methods FROM sessions WHERE token_hash = $1;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = $2, expires_at = $3 WHERE token_hash = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteOtherSessions :exec
-- Ends every session of a User but one, as changing the password does.
DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2;

-- name: DeleteSessionsOfUser :exec
-- Signing out from any Client signs out everywhere (docs/adr/0007).
DELETE FROM sessions WHERE user_id = $1;

-- name: ListUsers :many
SELECT id, email, name, password_hash, created_at FROM users ORDER BY lower(name), email;

-- name: SetSessionMethods :exec
-- A session that has just proved a Second Factor says so from then on.
UPDATE sessions SET methods = $2 WHERE token_hash = $1;

-- name: UpsertChallenge :exec
-- A User has one challenge. A new sign-in replaces its token and expiry; its wrong codes still
-- count until the window that began with the first of them has passed.
INSERT INTO sign_in_challenges (token_hash, user_id, expires_at, window_started_at) VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE SET
  token_hash = EXCLUDED.token_hash,
  passkey_ceremony = NULL,
  expires_at = EXCLUDED.expires_at,
  attempts = CASE WHEN sign_in_challenges.window_started_at <= sqlc.arg(window_since) THEN 0 ELSE sign_in_challenges.attempts END,
  window_started_at = CASE WHEN sign_in_challenges.window_started_at <= sqlc.arg(window_since) THEN EXCLUDED.window_started_at ELSE sign_in_challenges.window_started_at END;

-- name: SpendChallengeAttempt :one
-- Counts one code against a sign-in; past the last attempt nothing comes back.
UPDATE sign_in_challenges SET attempts = attempts + 1
WHERE token_hash = $1 AND attempts < $2 AND expires_at > $3
RETURNING user_id;

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
