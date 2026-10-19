-- name: LegacyState :one
-- Whether a Legacy Auth account (docs/adr/0006) is already here under its own id, and whether its email is taken.
SELECT EXISTS (SELECT 1 FROM users u WHERE u.id = sqlc.arg(id)::uuid)::boolean AS imported,
       EXISTS (SELECT 1 FROM users u WHERE u.email = sqlc.arg(email)::text)::boolean AS taken;

-- name: ImportUser :execrows
INSERT INTO users (id, email, name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING;
