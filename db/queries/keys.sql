-- name: LockKeys :exec
-- Held until the transaction ends, so of two replicas starting together one makes the key and the
-- other reads it.
SELECT pg_advisory_xact_lock(hashtext('identity-signing-keys'));

-- name: ListSigningKeys :many
SELECT id, public_key, sealed_private, signs_from, signs_until FROM signing_keys ORDER BY signs_from;

-- name: CreateSigningKey :exec
INSERT INTO signing_keys (id, public_key, sealed_private, signs_from, signs_until) VALUES ($1, $2, $3, $4, $5);

-- name: DeleteSigningKeysEndedBefore :exec
-- A key no token can still need: it stopped signing before a time.
DELETE FROM signing_keys WHERE signs_until < $1;

-- name: SealedSecret :one
SELECT sealed FROM sealed_secrets WHERE name = $1;

-- name: CreateSealedSecret :exec
INSERT INTO sealed_secrets (name, sealed, created_at) VALUES ($1, $2, $3);
