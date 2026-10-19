-- name: LockClientSecret :exec
-- Held until the transaction ends, so of two replicas seeing a new Client one makes its secret and
-- the other reads the hash.
SELECT pg_advisory_xact_lock(hashtext('identity-client-secret:' || sqlc.arg(client_id)::text));

-- name: ClientSecret :one
SELECT hash, previous_hash FROM client_secrets WHERE client_id = $1;

-- name: CreateClientSecret :exec
INSERT INTO client_secrets (client_id, hash, created_at) VALUES ($1, $2, $3);

-- name: RotateClientSecret :exec
-- The replaced secret stays accepted; if an earlier one still is, the Client has not used the
-- replaced one yet, so the earlier one is kept instead.
INSERT INTO client_secrets (client_id, hash, created_at) VALUES ($1, $2, $3)
ON CONFLICT (client_id) DO UPDATE
SET previous_hash = COALESCE(client_secrets.previous_hash, client_secrets.hash),
    hash = EXCLUDED.hash,
    rotated_at = EXCLUDED.created_at;

-- name: EndPreviousClientSecret :exec
-- Only while hash is still the current secret, so a rotation in between keeps its own previous.
UPDATE client_secrets SET previous_hash = NULL WHERE client_id = $1 AND hash = $2;

-- name: DeleteClientSecret :exec
DELETE FROM client_secrets WHERE client_id = $1;
