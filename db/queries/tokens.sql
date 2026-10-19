-- name: CreatePersonalToken :exec
INSERT INTO personal_tokens (id, user_id, client_id, name, token_hash, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: PersonalTokenByHash :one
SELECT id, user_id FROM personal_tokens WHERE token_hash = $1;

-- name: PersonalTokenByID :one
SELECT id, user_id, client_id, name, created_at, expires_at, revoked_at, last_used_at
FROM personal_tokens WHERE id = $1;

-- name: ListPersonalTokens :many
SELECT id, user_id, client_id, name, created_at, expires_at, revoked_at, last_used_at
FROM personal_tokens WHERE user_id = $1 ORDER BY created_at DESC, id;

-- name: RevokePersonalToken :execrows
-- Revokes a User's token once; a second revocation, or another User's token, changes nothing.
UPDATE personal_tokens SET revoked_at = $3 WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL;

-- name: TouchPersonalToken :exec
-- A token's last use is written at most once a minute, not on every check.
UPDATE personal_tokens SET last_used_at = $2
WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < $2::timestamptz - interval '1 minute');

-- name: CountPersonalTokensExpiring :one
-- Live tokens that expire after now and before a time.
SELECT count(*) FROM personal_tokens WHERE revoked_at IS NULL AND expires_at > $1 AND expires_at <= $2;

-- name: RevokeClientTokens :many
-- Revokes every live token for a Client at once, as turning step-up on does.
UPDATE personal_tokens SET revoked_at = $2
WHERE client_id = $1 AND revoked_at IS NULL AND expires_at > $2
RETURNING id, user_id;

-- name: CountLiveClientTokens :one
SELECT count(*) FROM personal_tokens WHERE client_id = $1 AND revoked_at IS NULL AND expires_at > $2;
