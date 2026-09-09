CREATE TABLE IF NOT EXISTS client_tenant_module_roles_dg_tmp (
    id TEXT PRIMARY KEY,
    client_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    module_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    assigned_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(client_id) REFERENCES clients(client_id) ON DELETE CASCADE,
    FOREIGN KEY(tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY(module_id) REFERENCES modules(id) ON DELETE CASCADE,
    FOREIGN KEY(role_id) REFERENCES roles(id) ON DELETE CASCADE,
    UNIQUE(client_id, tenant_id, module_id, role_id)
);

INSERT OR IGNORE INTO client_tenant_module_roles_dg_tmp (id, client_id, tenant_id, module_id, role_id, assigned_at)
SELECT id, client_id, tenant_id, module_id, role_id, assigned_at FROM client_tenant_module_roles;

DROP TABLE client_tenant_module_roles;

ALTER TABLE client_tenant_module_roles_dg_tmp RENAME TO client_tenant_module_roles;
