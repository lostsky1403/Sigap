-- packages/db/seed/rbac.sql
-- Dev-only seed data for RBAC identity module.
-- Run manually in local development after migrations are applied.
-- NEVER run in production. Contains only synthetic system roles and permissions.
-- No citizen health data, no real PII.

-- roles
INSERT INTO roles (name, description, is_system) VALUES
  ('super_admin', 'Full system access for platform administrators', true),
  ('facility_admin', 'Manage one specific healthcare facility''s settings and queues', true),
  ('operator', 'Generate and manage queue tickets at a facility', true),
  ('viewer', 'Read-only access to non-sensitive facility data', true)
ON CONFLICT (name) DO NOTHING;

-- permissions
INSERT INTO permissions (key, description) VALUES
  ('queue.generate', 'Generate a new queue ticket for a citizen'),
  ('queue.read', 'Read queue ticket information'),
  ('queue.manage', 'Manage queue status and operator actions'),
  ('facility.read', 'Read facility public data'),
  ('facility.manage', 'Update facility settings and bed counts'),
  ('audit.read', 'Query audit event logs')
ON CONFLICT (key) DO NOTHING;

-- role-permissions assignments
WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('super_admin', 'facility_admin', 'operator', 'viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('queue.generate', 'queue.read', 'queue.manage', 'facility.read', 'facility.manage', 'audit.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'super_admin'
ON CONFLICT (role_id, permission_id) DO NOTHING;

WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('facility_admin', 'operator', 'viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('queue.generate', 'queue.read', 'queue.manage', 'facility.read', 'facility.manage')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'facility_admin' AND p.key IN ('queue.generate', 'queue.read', 'queue.manage', 'facility.read', 'facility.manage')
ON CONFLICT (role_id, permission_id) DO NOTHING;

WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('operator', 'viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('queue.generate', 'queue.read', 'queue.manage', 'facility.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'operator' AND p.key IN ('queue.generate', 'queue.read', 'queue.manage', 'facility.read')
ON CONFLICT (role_id, permission_id) DO NOTHING;

WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('queue.read', 'facility.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'viewer' AND p.key IN ('queue.read', 'facility.read')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- New appointment and schedule permissions (forward-only, idempotent)
INSERT INTO permissions (key, description) VALUES
  ('appointment.read', 'View appointments and their status'),
  ('appointment.manage', 'Create, update, and cancel appointments; perform digital check-in'),
  ('schedule.read', 'View practitioner schedules and availability'),
  ('schedule.manage', 'Create and update practitioner schedules')
ON CONFLICT (key) DO NOTHING;

-- Assign new permissions to super_admin
WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('super_admin')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('appointment.read', 'appointment.manage', 'schedule.read', 'schedule.manage')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'super_admin'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Assign new permissions to facility_admin
WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('facility_admin')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('appointment.read', 'appointment.manage', 'schedule.read', 'schedule.manage')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'facility_admin'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Assign new permissions to operator (read + appointment.manage only)
WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('operator')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('appointment.read', 'appointment.manage', 'schedule.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'operator' AND p.key IN ('appointment.read', 'appointment.manage', 'schedule.read')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Assign new permissions to viewer (read only)
WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('appointment.read', 'schedule.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'viewer' AND p.key IN ('appointment.read', 'schedule.read')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Notification outbox permissions (forward-only, idempotent).
-- Applied additively; existing rows are NOT modified.
-- ---------------------------------------------------------------------------

-- New permission keys (insert-if-missing via ON CONFLICT).
INSERT INTO permissions (key, description) VALUES
  ('notification.read', 'View the notification outbox, masked recipients, and delivery attempts'),
  ('notification.manage', 'Retry or cancel pending/failed notifications in the outbox')
ON CONFLICT (key) DO NOTHING;

-- super_admin and facility_admin get BOTH notification.read and notification.manage.
WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('super_admin','facility_admin')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('notification.read','notification.manage')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name IN ('super_admin','facility_admin')
  AND p.key IN ('notification.read','notification.manage')
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- operator gets notification.read only (view outbox, cannot retry/cancel).
WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('operator')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('notification.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'operator' AND p.key = 'notification.read'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- viewer gets notification.read only (read-only view).
WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('notification.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'viewer' AND p.key = 'notification.read'
ON CONFLICT (role_id, permission_id) DO NOTHING;
