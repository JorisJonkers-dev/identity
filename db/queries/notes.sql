-- name: CreateNote :one
INSERT INTO notes (text) VALUES ($1)
RETURNING id, text, created_at;

-- name: ListNotes :many
SELECT id, text, created_at
FROM notes
ORDER BY created_at DESC, id
LIMIT $1;
