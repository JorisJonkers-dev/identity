-- name: CreateUser :exec
INSERT INTO users (id, email, name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5);

-- name: UserByEmail :one
SELECT id, email, name, password_hash, confirmed_at, created_at FROM users WHERE email = $1;

-- name: UserByID :one
SELECT id, email, name, password_hash, confirmed_at, created_at FROM users WHERE id = $1;

-- name: ConfirmUser :exec
UPDATE users SET confirmed_at = $2 WHERE id = $1 AND confirmed_at IS NULL;

-- name: UpdateUserName :exec
UPDATE users SET name = $2 WHERE id = $1;

-- name: UpdateUserPassword :exec
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: CreateConfirmation :exec
INSERT INTO email_confirmations (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: TakeConfirmation :one
-- A confirmation is single use: taking it deletes it, whether or not it has expired.
DELETE FROM email_confirmations WHERE token_hash = $1 RETURNING user_id, expires_at;

-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at) VALUES ($1, $2, $3, $3, $4);

-- name: SessionByHash :one
SELECT token_hash, user_id, last_seen_at, expires_at FROM sessions WHERE token_hash = $1;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = $2, expires_at = $3 WHERE token_hash = $1;

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token_hash = $1;
