-- name: CreateRegistration :exec
INSERT INTO oauth_registrations (client_id, name, redirect_uris, created_at) VALUES ($1, $2, $3, $4);

-- name: RegistrationByID :one
SELECT client_id, name, redirect_uris, created_at, last_used_at FROM oauth_registrations WHERE client_id = $1;

-- name: UseRegistration :exec
UPDATE oauth_registrations SET last_used_at = $2 WHERE client_id = $1;

-- name: PruneRegistrations :execrows
-- A registration without a Connected App goes a day after it was made when nobody consented to
-- it, and once unused for as long as a refresh token lasts when somebody did.
DELETE FROM oauth_registrations r
WHERE (r.last_used_at IS NULL AND r.created_at < sqlc.arg(unconsented)::timestamptz
       OR r.last_used_at < sqlc.arg(idle)::timestamptz)
  AND NOT EXISTS (SELECT 1 FROM connected_apps c WHERE c.registration_id = r.client_id);

-- name: MCPResources :many
SELECT client_id FROM mcp_resources ORDER BY client_id;

-- name: IsMCPResource :one
SELECT EXISTS (SELECT 1 FROM mcp_resources WHERE client_id = $1)::boolean AS mcp;

-- name: MarkMCPResource :exec
INSERT INTO mcp_resources (client_id, created_at) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: UnmarkMCPResource :exec
DELETE FROM mcp_resources WHERE client_id = $1;

-- name: ConsentToConnectedApp :one
-- The Connected App for a consent, the one already there when the User consented before.
INSERT INTO connected_apps (id, registration_id, user_id, client_id, created_at) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT ON CONSTRAINT connected_apps_one_per_consent DO UPDATE SET registration_id = EXCLUDED.registration_id
RETURNING id;

-- name: ConnectedAppFor :one
SELECT id FROM connected_apps WHERE registration_id = $1 AND user_id = $2 AND client_id = $3;

-- name: ConnectedAppsOfUser :many
SELECT c.id, c.client_id, c.created_at, c.last_used_at, r.name FROM connected_apps c
JOIN oauth_registrations r ON r.client_id = c.registration_id
WHERE c.user_id = $1 ORDER BY c.created_at, c.id;

-- name: ConnectedAppByID :one
SELECT c.id, c.user_id, c.client_id, c.created_at, c.last_used_at, r.name FROM connected_apps c
JOIN oauth_registrations r ON r.client_id = c.registration_id
WHERE c.id = $1;

-- name: DeleteConnectedApp :execrows
DELETE FROM connected_apps WHERE id = $1;

-- name: ConnectedAppsForClient :many
-- Every Connected App of a Client, locked, as turning step-up on removes them in one transaction.
SELECT c.id, c.user_id, r.name FROM connected_apps c
JOIN oauth_registrations r ON r.client_id = c.registration_id
WHERE c.client_id = $1 ORDER BY c.id FOR UPDATE OF c;

-- name: DeleteConnectedAppsForClient :exec
DELETE FROM connected_apps WHERE client_id = $1;

-- name: CountConnectedAppsForClient :one
SELECT count(*) FROM connected_apps WHERE client_id = $1;

-- name: TouchConnectedApp :exec
UPDATE connected_apps SET last_used_at = $2 WHERE id = $1;

-- name: CreateConnectedAppToken :exec
INSERT INTO connected_app_tokens (id, connected_app_id, kind, hash, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6);

-- name: ConnectedAppAccess :one
-- A live access token, found by the id its sealed form carries.
SELECT t.id, t.expires_at, c.id AS app_id, c.user_id, c.client_id, c.registration_id FROM connected_app_tokens t
JOIN connected_apps c ON c.id = t.connected_app_id
WHERE t.id = $1 AND t.kind = 'access';

-- name: DeleteConnectedAppAccess :exec
DELETE FROM connected_app_tokens WHERE id = $1 AND kind = 'access';

-- name: ConnectedAppRefresh :one
SELECT t.id, t.expires_at, t.used_at, c.id AS app_id, c.user_id, c.client_id, c.registration_id FROM connected_app_tokens t
JOIN connected_apps c ON c.id = t.connected_app_id
WHERE t.hash = $1 AND t.kind = 'refresh';

-- name: SpendRefresh :execrows
-- A refresh token is spent once; a second spend finds it used and changes nothing.
UPDATE connected_app_tokens SET used_at = $2 WHERE id = $1 AND used_at IS NULL;

-- name: AccessTokensOfClient :many
SELECT t.id FROM connected_app_tokens t
JOIN connected_apps c ON c.id = t.connected_app_id
WHERE c.client_id = $1 AND t.kind = 'access';

-- name: AccessTokensOfApp :many
SELECT id FROM connected_app_tokens WHERE connected_app_id = $1 AND kind = 'access';

-- name: PruneConnectedAppTokens :execrows
DELETE FROM connected_app_tokens WHERE expires_at < $1;
