-- name: ListGroups :many
SELECT id, name, admins FROM groups ORDER BY admins DESC, lower(name);

-- name: GroupByID :one
SELECT id, name, admins FROM groups WHERE id = $1;

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

-- name: AddMember :exec
INSERT INTO memberships (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING;

-- name: RemoveMember :exec
DELETE FROM memberships WHERE group_id = $1 AND user_id = $2;

-- name: IsAdmin :one
SELECT EXISTS (
  SELECT 1 FROM memberships m JOIN groups g ON g.id = m.group_id WHERE g.admins AND m.user_id = $1
);

-- name: ListAssignments :many
SELECT group_id, client_id, role FROM assignments ORDER BY client_id, role;

-- name: Assign :exec
INSERT INTO assignments (group_id, client_id, role) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING;

-- name: Unassign :exec
DELETE FROM assignments WHERE group_id = $1 AND client_id = $2 AND role = $3;

-- name: RolesOf :many
-- A User's App Roles on a Client: the union over their Groups (docs/adr/0004).
SELECT DISTINCT a.role FROM assignments a JOIN memberships m ON m.group_id = a.group_id
WHERE m.user_id = $1 AND a.client_id = $2 ORDER BY a.role;

-- name: RolesByClient :many
-- Every App Role a User holds, per Client: the union over their Groups (docs/adr/0004).
SELECT DISTINCT a.client_id, a.role FROM assignments a JOIN memberships m ON m.group_id = a.group_id
WHERE m.user_id = $1 ORDER BY a.client_id, a.role;

-- name: AdminsGroup :one
SELECT id, name, admins FROM groups WHERE admins;

-- name: CountAdmins :one
SELECT count(*) FROM memberships m JOIN groups g ON g.id = m.group_id WHERE g.admins;

-- name: CountUnassigned :one
-- Confirmed Users in no Group, the admins Group included (docs/adr/0005).
SELECT count(*) FROM users u WHERE NOT EXISTS (SELECT 1 FROM memberships m WHERE m.user_id = u.id);
