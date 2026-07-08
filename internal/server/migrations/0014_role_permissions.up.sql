-- Align system role permissions with the actual permission constants used by the
-- backend enforcement layer (rbac.go). The original seeded values used legacy
-- strings that don't match any enforced permission.

UPDATE roles SET
    description = 'Full access to all resources and settings',
    permissions  = '["*"]'
WHERE name = 'owner' AND is_system = true;

UPDATE roles SET
    description = 'All write actions: policies, alerts, jobs, ownership, team, API keys, reports, and audit',
    permissions  = '["policies:manage","alerts:manage","jobs:manage","ownership:manage","team:manage","apikeys:manage","reports:manage","audit:view"]'
WHERE name = 'admin' AND is_system = true;

UPDATE roles SET
    description = 'View reports and audit logs - no policy or alert writes',
    permissions  = '["reports:manage","audit:view"]'
WHERE name = 'billing' AND is_system = true;

UPDATE roles SET
    description = 'Manage job data (snooze, archive, delete runs)',
    permissions  = '["jobs:manage"]'
WHERE name = 'developer' AND is_system = true;

UPDATE roles SET
    description = 'Read-only access to all dashboards',
    permissions  = '[]'
WHERE name = 'viewer' AND is_system = true;
