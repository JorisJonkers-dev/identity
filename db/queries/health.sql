-- name: Ping :one
-- Readiness: the database answers a query, not only a connection.
SELECT 1::int AS ok;
