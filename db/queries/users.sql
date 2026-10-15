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
INSERT INTO sessions (token_hash, id, user_id, created_at, last_seen_at, expires_at) VALUES ($1, $2, $3, $4, $4, $5);

-- name: SessionByHash :one
SELECT token_hash, id, user_id, created_at, last_seen_at, expires_at FROM sessions WHERE token_hash = $1;

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
