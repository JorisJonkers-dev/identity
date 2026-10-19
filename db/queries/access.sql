-- name: ListGroups :many
SELECT id, name, admins, requires_two_factor FROM groups ORDER BY admins DESC, lower(name);

-- name: GroupByID :one
SELECT id, name, admins, requires_two_factor FROM groups WHERE id = $1;

-- name: CreateGroup :exec
INSERT INTO groups (id, name) VALUES ($1, $2);

-- name: RenameGroup :execrows
UPDATE groups SET name = $2 WHERE id = $1;

-- name: DeleteGroup :exec
-- The admins Group stays, whatever the caller checked.
DELETE FROM groups WHERE id = $1 AND NOT admins;

-- name: LockGroup :one
-- Holds the Group until the transaction ends, so two removals cannot both see the other member.
SELECT admins FROM groups WHERE id = $1 FOR UPDATE;

-- name: CountMembers :one
SELECT count(*) FROM memberships WHERE group_id = $1;

-- name: ListMemberships :many
SELECT group_id, user_id FROM memberships;

-- name: AddMember :execrows
INSERT INTO memberships (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveMember :execrows
DELETE FROM memberships WHERE group_id = $1 AND user_id = $2;

-- name: IsAdmin :one
SELECT EXISTS (
  SELECT 1 FROM memberships m JOIN groups g ON g.id = m.group_id WHERE g.admins AND m.user_id = $1
);

-- name: ListAssignments :many
SELECT group_id::uuid AS group_id, client_id, role FROM assignments WHERE group_id IS NOT NULL ORDER BY client_id, role;

-- name: ListUserAssignments :many
SELECT user_id::uuid AS user_id, client_id, role FROM assignments WHERE user_id IS NOT NULL ORDER BY client_id, role;

-- name: Assign :execrows
INSERT INTO assignments (group_id, client_id, role) VALUES (sqlc.arg(group_id)::uuid, sqlc.arg(client_id), sqlc.arg(role)) ON CONFLICT DO NOTHING;

-- name: Unassign :execrows
DELETE FROM assignments WHERE group_id = sqlc.arg(group_id)::uuid AND client_id = sqlc.arg(client_id) AND role = sqlc.arg(role);

-- name: AssignUser :execrows
INSERT INTO assignments (user_id, client_id, role) VALUES (sqlc.arg(user_id)::uuid, sqlc.arg(client_id), sqlc.arg(role)) ON CONFLICT DO NOTHING;

-- name: UnassignUser :execrows
DELETE FROM assignments WHERE user_id = sqlc.arg(user_id)::uuid AND client_id = sqlc.arg(client_id) AND role = sqlc.arg(role);

-- name: RolesOf :many
-- A User's App Roles on a Client: their own and their Groups', combined (docs/adr/0018), less a
-- Two-factor Group's while they have no two-factor.
SELECT DISTINCT a.role FROM assignments a
WHERE a.client_id = sqlc.arg(client_id)
  AND (a.user_id = sqlc.arg(user_id)::uuid OR a.group_id IN (
    SELECT m.group_id FROM memberships m JOIN groups g ON g.id = m.group_id
    WHERE m.user_id = sqlc.arg(user_id)::uuid AND (NOT g.requires_two_factor
      OR EXISTS (SELECT 1 FROM totp_factors f WHERE f.user_id = m.user_id)
      OR EXISTS (SELECT 1 FROM passkeys p WHERE p.user_id = m.user_id))))
ORDER BY a.role;

-- name: RolesByClient :many
-- Every App Role a User holds, per Client, as RolesOf counts them.
SELECT DISTINCT a.client_id, a.role FROM assignments a
WHERE a.user_id = sqlc.arg(user_id)::uuid OR a.group_id IN (
  SELECT m.group_id FROM memberships m JOIN groups g ON g.id = m.group_id
  WHERE m.user_id = sqlc.arg(user_id)::uuid AND (NOT g.requires_two_factor
    OR EXISTS (SELECT 1 FROM totp_factors f WHERE f.user_id = m.user_id)
    OR EXISTS (SELECT 1 FROM passkeys p WHERE p.user_id = m.user_id)))
ORDER BY a.client_id, a.role;

-- name: AdminsGroup :one
SELECT id, name, admins, requires_two_factor FROM groups WHERE admins;

-- name: CountAdmins :one
SELECT count(*) FROM memberships m JOIN groups g ON g.id = m.group_id WHERE g.admins;

-- name: CountUnassigned :one
-- Confirmed Users who hold no App Role by any route, Admins aside (docs/adr/0005, 0018).
SELECT count(*) FROM users u WHERE NOT EXISTS (
  SELECT 1 FROM assignments a
  WHERE a.user_id = u.id OR a.group_id IN (SELECT m.group_id FROM memberships m WHERE m.user_id = u.id)
) AND NOT EXISTS (SELECT 1 FROM memberships m JOIN groups g ON g.id = m.group_id WHERE g.admins AND m.user_id = u.id);

-- name: ListUnassigned :many
-- The Users CountUnassigned counts.
SELECT u.id FROM users u WHERE NOT EXISTS (
  SELECT 1 FROM assignments a
  WHERE a.user_id = u.id OR a.group_id IN (SELECT m.group_id FROM memberships m WHERE m.user_id = u.id)
) AND NOT EXISTS (SELECT 1 FROM memberships m JOIN groups g ON g.id = m.group_id WHERE g.admins AND m.user_id = u.id);

-- name: SetGroupTwoFactor :execrows
-- The admins Group always requires two-factor; the table refuses otherwise.
UPDATE groups SET requires_two_factor = $2 WHERE id = $1;
