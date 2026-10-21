-- name: CreateAuthRequest :exec
INSERT INTO auth_requests (id, client_id, redirect_uri, scopes, state, nonce, response_type, response_mode,
    code_challenge, code_challenge_method, created_at, expires_at, resource)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);

-- name: AuthRequestByID :one
SELECT * FROM auth_requests WHERE id = $1;

-- name: TakeAuthRequestByCodeHash :one
-- Redeeming a code removes its request in the same statement, so two exchanges racing with one
-- code cannot both get tokens.
DELETE FROM auth_requests WHERE code_hash = $1 RETURNING *;

-- name: CompleteAuthRequest :execrows
-- Records who signed in, once: a request already completed keeps its first User.
UPDATE auth_requests SET user_id = $2, session_id = $3, auth_time = $4, methods = $5, two_factor_at = $6 WHERE id = $1 AND user_id IS NULL;

-- name: SaveAuthCode :execrows
-- A request gets one code; a second is refused.
UPDATE auth_requests SET code_hash = $2 WHERE id = $1 AND code_hash IS NULL AND user_id IS NOT NULL;

-- name: DeleteAuthRequest :exec
DELETE FROM auth_requests WHERE id = $1;

-- name: AddSessionClient :exec
INSERT INTO session_clients (session_id, client_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: TakeSessionClients :many
-- The Clients the ended sessions signed into, removed as they are read.
DELETE FROM session_clients WHERE session_id = ANY(sqlc.arg(session_ids)::uuid[]) RETURNING session_id, client_id;

-- name: TakeUserSessionClients :many
-- A User's live sessions that signed into the Clients, removed as they are read.
DELETE FROM session_clients sc USING sessions s
WHERE sc.session_id = s.id AND s.user_id = sqlc.arg(user_id)::uuid AND sc.client_id = ANY(sqlc.arg(client_ids)::text[])
RETURNING sc.session_id, sc.client_id;

-- name: StepUpMinutes :one
SELECT minutes FROM step_up WHERE client_id = $1;

-- name: SetStepUp :exec
INSERT INTO step_up (client_id, minutes, changed_at) VALUES ($1, $2, $3)
ON CONFLICT (client_id) DO UPDATE SET minutes = EXCLUDED.minutes, changed_at = EXCLUDED.changed_at;

-- name: ClearStepUp :exec
DELETE FROM step_up WHERE client_id = $1;
