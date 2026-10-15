-- +goose Up
-- A migration waits at most 5s for a lock and runs at most 60s, so it fails rather than stalls.
SET lock_timeout = '5s';
SET statement_timeout = '60s';

-- Integrity lives in the database (docs/blueprints/go-api.md §8.2): the length rule the domain
-- checks is a CHECK here too, so no write path stores text longer than the domain allows.
CREATE TABLE IF NOT EXISTS notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    text text NOT NULL CHECK (char_length(text) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- A plain CREATE INDEX on a table made in the same transaction blocks nobody.
CREATE INDEX IF NOT EXISTS notes_created_at_idx ON notes (created_at DESC);
