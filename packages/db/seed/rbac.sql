-- packages/db/seed/rbac.sql
-- Dev-only seed data for RBAC identity module.
-- Run manually in local development after migrations are applied.
-- NEVER run in production. Contains only synthetic system roles and permissions.
-- No citizen health data, no real PII.

-- roles
INSERT INTO roles (name, description, is_system) VALUES
  ('super_admin', 'Full system access for platform administrators', true),
  ('facility_admin', 'Manage one specific healthcare facility\'s settings and queues', true),
  ('operator', 'Generate and manage queue tickets at a facility', true),
  ('viewer', 'Read-only access to non-sensitive facility data', true);

-- permissions
INSERT INTO permissions (key, description) VALUES
  ('queue.generate', 'Generate a new queue ticket for a citizen'),
  ('queue.read', 'Read queue ticket information'),
  ('facility.read', 'Read facility public data'),
  ('facility.manage', 'Update facility settings and bed counts'),
  ('audit.read', 'Query audit event logs');

-- role-permissions assignments
WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('super_admin', 'facility_admin', 'operator', 'viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('queue.generate', 'queue.read', 'facility.read', 'facility.manage', 'audit.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'super_admin';

WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('facility_admin', 'operator', 'viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('queue.generate', 'queue.read', 'facility.read', 'facility.manage')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'facility_admin' AND p.key IN ('queue.generate', 'queue.read', 'facility.read', 'facility.manage');

WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('operator', 'viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('queue.generate', 'queue.read', 'facility.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'operator' AND p.key IN ('queue.generate', 'queue.read', 'facility.read');

WITH role_ids AS (
  SELECT id, name FROM roles WHERE name IN ('viewer')
),
perm_ids AS (
  SELECT id, key FROM permissions WHERE key IN ('queue.read', 'facility.read')
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id FROM role_ids r CROSS JOIN perm_ids p
WHERE r.name = 'viewer' AND p.key IN ('queue.read', 'facility.read');
