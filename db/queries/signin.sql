-- name: CreateAuthRequest :exec
INSERT INTO auth_requests (id, client_id, redirect_uri, scopes, state, nonce, response_type, response_mode,
    code_challenge, code_challenge_method, created_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: AuthRequestByID :one
SELECT * FROM auth_requests WHERE id = $1;

-- name: AuthRequestByCodeHash :one
SELECT * FROM auth_requests WHERE code_hash = $1;

-- name: CompleteAuthRequest :execrows
-- Records who signed in, once: a request already completed keeps its first User.
UPDATE auth_requests SET user_id = $2, session_id = $3, auth_time = $4 WHERE id = $1 AND user_id IS NULL;

-- name: SaveAuthCode :execrows
-- A request gets one code; a second is refused.
UPDATE auth_requests SET code_hash = $2 WHERE id = $1 AND code_hash IS NULL AND user_id IS NOT NULL;

-- name: DeleteAuthRequest :exec
DELETE FROM auth_requests WHERE id = $1;
