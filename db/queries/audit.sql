-- name: AppendAuditEvent :exec
INSERT INTO audit_events (id, at, kind, action, actor_id, user_id, client_id, group_id, group_name, role, email, reason, minutes, machine_id, machine_name)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);

-- name: ListAuditEvents :many
-- Newest first, before a (at, id) cursor; a User filter matches what they did and what was done to them.
SELECT id, at, kind, action, actor_id, user_id, client_id, group_id, group_name, role, email, reason, minutes, machine_id, machine_name
FROM audit_events
WHERE (sqlc.narg(user_id)::uuid IS NULL OR user_id = sqlc.narg(user_id) OR actor_id = sqlc.narg(user_id))
  AND (sqlc.narg(client_id)::text IS NULL OR client_id = sqlc.narg(client_id))
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
  AND (sqlc.narg(before_at)::timestamptz IS NULL OR (at, id) < (sqlc.narg(before_at), sqlc.narg(before_id)::uuid))
ORDER BY at DESC, id DESC
LIMIT sqlc.arg(max);

-- name: DeleteAuditEventsBefore :execrows
DELETE FROM audit_events WHERE at < $1;

-- name: CountSignInsByHour :many
-- Sign-ins to the Identity Provider itself, and failed ones, per hour since a time.
SELECT date_trunc('hour', at)::timestamptz AS hour,
       count(*) FILTER (WHERE action = 'signed-in') AS signed_in,
       count(*) FILTER (WHERE action = 'sign-in-failed') AS failed
FROM audit_events
WHERE kind = 'sign-in' AND client_id IS NULL AND at >= $1
GROUP BY 1
ORDER BY 1;
