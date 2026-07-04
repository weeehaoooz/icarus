-- Initialize initial schema
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    email TEXT,
    first_name TEXT,
    last_name TEXT,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS clients (
    client_id TEXT PRIMARY KEY,
    public_key TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    token TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    code TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS modules (
    id TEXT PRIMARY KEY,
    code TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    base_url TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS permissions (
    id TEXT PRIMARY KEY,
    module_id TEXT NOT NULL,
    action TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(module_id) REFERENCES modules(id) ON DELETE CASCADE,
    UNIQUE(module_id, action)
);

CREATE TABLE IF NOT EXISTS roles (
    id TEXT PRIMARY KEY,
    module_id TEXT NOT NULL,
    tenant_id TEXT,
    app_code TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    description TEXT,
    is_system_role BOOLEAN NOT NULL DEFAULT FALSE,
    type TEXT NOT NULL DEFAULT 'Custom',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(module_id) REFERENCES modules(id) ON DELETE CASCADE,
    FOREIGN KEY(tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    UNIQUE(tenant_id, module_id, name, app_code)
);

CREATE TABLE IF NOT EXISTS role_permissions (
    role_id TEXT NOT NULL,
    permission_id TEXT NOT NULL,
    PRIMARY KEY (role_id, permission_id),
    FOREIGN KEY(role_id) REFERENCES roles(id) ON DELETE CASCADE,
    FOREIGN KEY(permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS user_tenant_module_roles (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL,
    tenant_id TEXT NOT NULL,
    module_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY(tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY(module_id) REFERENCES modules(id) ON DELETE CASCADE,
    FOREIGN KEY(role_id) REFERENCES roles(id) ON DELETE CASCADE,
    UNIQUE(user_id, tenant_id, module_id)
);

CREATE TABLE IF NOT EXISTS client_tenant_module_roles (
    id TEXT PRIMARY KEY,
    client_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    module_id TEXT NOT NULL,
    role_id TEXT NOT NULL,
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(client_id) REFERENCES clients(client_id) ON DELETE CASCADE,
    FOREIGN KEY(tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY(module_id) REFERENCES modules(id) ON DELETE CASCADE,
    FOREIGN KEY(role_id) REFERENCES roles(id) ON DELETE CASCADE,
    UNIQUE(client_id, tenant_id, module_id)
);

CREATE TABLE IF NOT EXISTS applications (
    id TEXT PRIMARY KEY,
    code TEXT UNIQUE NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS module_applications (
    module_id TEXT NOT NULL,
    app_code TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (module_id, app_code)
);

CREATE TABLE IF NOT EXISTS app_centric_role_templates (
    id TEXT PRIMARY KEY,
    module_id TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(module_id) REFERENCES modules(id) ON DELETE CASCADE,
    UNIQUE(module_id, name)
);

CREATE TABLE IF NOT EXISTS app_centric_role_template_permissions (
    template_id TEXT NOT NULL,
    permission_id TEXT NOT NULL,
    PRIMARY KEY (template_id, permission_id),
    FOREIGN KEY(template_id) REFERENCES app_centric_role_templates(id) ON DELETE CASCADE,
    FOREIGN KEY(permission_id) REFERENCES permissions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS ldap_config (
    id INTEGER PRIMARY KEY DEFAULT 1,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    server_url TEXT NOT NULL DEFAULT '',
    bind_dn TEXT NOT NULL DEFAULT '',
    bind_password TEXT NOT NULL DEFAULT '',
    search_base TEXT NOT NULL DEFAULT '',
    username_attribute TEXT NOT NULL DEFAULT 'uid',
    mail_attribute TEXT NOT NULL DEFAULT 'mail',
    first_name_attribute TEXT NOT NULL DEFAULT 'givenName',
    last_name_attribute TEXT NOT NULL DEFAULT 'sn',
    CHECK (id = 1)
);

CREATE TABLE IF NOT EXISTS user_groups (
    user_id INTEGER NOT NULL,
    group_name TEXT NOT NULL,
    group_type TEXT NOT NULL, -- 'LDAP' or 'Custom'
    PRIMARY KEY (user_id, group_name),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS role_hierarchy (
    parent_role_id TEXT NOT NULL,
    child_role_id TEXT NOT NULL,
    PRIMARY KEY (parent_role_id, child_role_id),
    FOREIGN KEY (parent_role_id) REFERENCES roles(id) ON DELETE CASCADE,
    FOREIGN KEY (child_role_id) REFERENCES roles(id) ON DELETE CASCADE
);

-- Seed baseline data
INSERT INTO tenants (id, code, name, status) VALUES
    ('system-tenant', 'system', 'System Tenant', 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO modules (id, code, name, base_url, is_active) VALUES
    ('icarus-auth-ms', 'icarus-auth-ms', 'Auth Service', 'http://localhost:8080', TRUE),
    ('icarus-admin-ms', 'icarus-admin-ms', 'Platform Service', 'http://localhost:8081', TRUE)
ON CONFLICT (id) DO NOTHING;

-- Seed permissions for icarus-auth-ms
INSERT INTO permissions (id, module_id, action, description) VALUES
    ('icarus-auth-ms:manage:users', 'icarus-auth-ms', 'manage:users', 'Ability to manage users'),
    ('icarus-auth-ms:manage:clients', 'icarus-auth-ms', 'manage:clients', 'Ability to manage clients'),
    ('icarus-auth-ms:manage:roles', 'icarus-auth-ms', 'manage:roles', 'Ability to manage roles'),
    ('icarus-auth-ms:manage:permissions', 'icarus-auth-ms', 'manage:permissions', 'Ability to view permissions')
ON CONFLICT (id) DO NOTHING;

-- Seed permissions for icarus-admin-ms
INSERT INTO permissions (id, module_id, action, description) VALUES
    ('icarus-admin-ms:manage:sites', 'icarus-admin-ms', 'manage:sites', 'Ability to manage sites'),
    ('icarus-admin-ms:manage:apis', 'icarus-admin-ms', 'manage:apis', 'Ability to manage apis')
ON CONFLICT (id) DO NOTHING;

-- Seed admin role for icarus-auth-ms (system global)
INSERT INTO roles (id, module_id, tenant_id, name, description, is_system_role, type) VALUES
    ('icarus-auth-ms:admin', 'icarus-auth-ms', NULL, 'admin', 'Administrator with full access', TRUE, 'Custom')
ON CONFLICT (id) DO NOTHING;

-- Map permissions to icarus-auth-ms admin role
INSERT INTO role_permissions (role_id, permission_id) VALUES
    ('icarus-auth-ms:admin', 'icarus-auth-ms:manage:users'),
    ('icarus-auth-ms:admin', 'icarus-auth-ms:manage:clients'),
    ('icarus-auth-ms:admin', 'icarus-auth-ms:manage:roles'),
    ('icarus-auth-ms:admin', 'icarus-auth-ms:manage:permissions')
ON CONFLICT (role_id, permission_id) DO NOTHING;
