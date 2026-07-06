-- Lightweight roles catalogue for the workflow service to query.
-- Populated/updated when a module registers via /api/v1/governance/modules/register.
CREATE TABLE IF NOT EXISTS roles (
    id          TEXT PRIMARY KEY,
    module_id   TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT,
    is_system   BOOLEAN NOT NULL DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(module_id, name),
    FOREIGN KEY(module_id) REFERENCES modules(id) ON DELETE CASCADE
);

-- Seed the built-in admin role
INSERT OR IGNORE INTO roles (id, module_id, name, description, is_system)
SELECT 'role-admin-001', id, 'admin', 'Platform administrator', 1
FROM modules LIMIT 1;

CREATE INDEX IF NOT EXISTS idx_roles_module_id ON roles(module_id);
CREATE INDEX IF NOT EXISTS idx_roles_name      ON roles(name);
