-- name: CreateGateCode :exec
INSERT INTO gate_codes (code_hash, host, session_id, state_hash, return_path, expires_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: TakeGateCode :one
-- Redeeming a code removes it in the same statement, so a code works once.
DELETE FROM gate_codes WHERE code_hash = $1 RETURNING host, session_id, state_hash, return_path, expires_at;

-- name: CreateGateSession :exec
INSERT INTO gate_sessions (token_hash, host, session_id, created_at) VALUES ($1, $2, $3, $4);

-- name: GateSessionByHash :one
SELECT host, session_id FROM gate_sessions WHERE token_hash = $1;
