-- Drop old tables
DROP TABLE IF EXISTS approver_definitions;
DROP TABLE IF EXISTS workflow_stage_definitions;
DROP TABLE IF EXISTS workflow_steps;
DROP TABLE IF EXISTS workflow_instances;
DROP TABLE IF EXISTS role_workflow_mappings;
DROP TABLE IF EXISTS workflow_definitions;
DROP TABLE IF EXISTS access_cart_items;
DROP TABLE IF EXISTS access_carts;

-- Create workflows table (instruction sets)
CREATE TABLE IF NOT EXISTS workflows (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    definition_key TEXT NOT NULL,
    version        INTEGER NOT NULL DEFAULT 1,
    is_current     BOOLEAN NOT NULL DEFAULT 1,
    status         TEXT NOT NULL DEFAULT 'DRAFT', -- DRAFT | ACTIVE | SUPERSEDED | ARCHIVED
    supersedes_id  TEXT,
    metadata       TEXT, -- JSON config
    created_by     TEXT NOT NULL,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(supersedes_id) REFERENCES workflows(id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_workflows_current
    ON workflows(definition_key) WHERE is_current = 1;

CREATE INDEX IF NOT EXISTS idx_workflows_key ON workflows(definition_key);

-- Create workflow_nodes table (nodes in the instruction set)
CREATE TABLE IF NOT EXISTS workflow_nodes (
    id               TEXT NOT NULL,
    workflow_id      TEXT NOT NULL,
    name             TEXT NOT NULL,
    type             TEXT NOT NULL, -- APPROVAL | OPERATION | etc.
    depends_on       TEXT NOT NULL, -- JSON array of strings
    max_retries      INTEGER NOT NULL DEFAULT 0,
    retry_interval   INTEGER NOT NULL DEFAULT 0,
    config           TEXT,          -- JSON map of type-specific properties
    PRIMARY KEY (id, workflow_id),
    FOREIGN KEY(workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_workflow_nodes_wf ON workflow_nodes(workflow_id);

-- Create role_workflow_mappings table
CREATE TABLE IF NOT EXISTS role_workflow_mappings (
    id             TEXT PRIMARY KEY,
    role_id                TEXT NOT NULL UNIQUE,
    workflow_id TEXT NOT NULL,
    is_active              BOOLEAN NOT NULL DEFAULT 1,
    effective_from         DATETIME DEFAULT CURRENT_TIMESTAMP,
    effective_to           DATETIME,
    created_by             TEXT NOT NULL,
    created_at             DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(workflow_id) REFERENCES workflows(id)
);

CREATE INDEX IF NOT EXISTS idx_role_workflow_mapping_role
    ON role_workflow_mappings(role_id, is_active);

-- Recreate access_carts
CREATE TABLE IF NOT EXISTS access_carts (
    id            TEXT PRIMARY KEY,
    requester_id  TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'DRAFT', -- DRAFT | SUBMITTED | IN_PROGRESS | COMPLETED | CANCELLED
    justification TEXT,
    submitted_at  DATETIME,
    completed_at  DATETIME,
    version       INTEGER NOT NULL DEFAULT 1,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Recreate access_cart_items referencing execution_id
CREATE TABLE IF NOT EXISTS access_cart_items (
    id           TEXT PRIMARY KEY,
    cart_id      TEXT NOT NULL,
    role_id      TEXT NOT NULL,
    role_name    TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'PENDING', -- PENDING | IN_PROGRESS | APPROVED | REJECTED | CANCELLED
    execution_id TEXT, -- replaces workflow_instance_id
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(cart_id) REFERENCES access_carts(id) ON DELETE CASCADE
);

-- Create executions table (runs)
CREATE TABLE IF NOT EXISTS executions (
    id           TEXT PRIMARY KEY,
    workflow_id  TEXT NOT NULL, -- replaces workflow_definition_id
    cart_item_id TEXT NOT NULL UNIQUE,
    status       TEXT NOT NULL DEFAULT 'IN_PROGRESS', -- IN_PROGRESS | COMPLETED | REJECTED | CANCELLED | ERROR
    error_message TEXT,
    started_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    version      INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(workflow_id) REFERENCES workflows(id),
    FOREIGN KEY(cart_item_id) REFERENCES access_cart_items(id)
);

-- Create execution_nodes table (execution steps)
CREATE TABLE IF NOT EXISTS execution_nodes (
    id                  TEXT PRIMARY KEY,
    execution_id        TEXT NOT NULL,
    node_id             TEXT NOT NULL,
    node_name           TEXT NOT NULL,
    assigned_to_user_id TEXT,
    assigned_to_role    TEXT,
    status              TEXT NOT NULL DEFAULT 'PENDING', -- PENDING | APPROVED | REJECTED | DELEGATED | CANCELLED
    decision_comment    TEXT,
    acted_by_user_id    TEXT,
    assigned_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
    acted_at            DATETIME,
    retry_attempt       INTEGER NOT NULL DEFAULT 0,
    version             INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(execution_id) REFERENCES executions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_execution_nodes_node ON execution_nodes(node_id);
CREATE INDEX IF NOT EXISTS idx_execution_nodes_exec ON execution_nodes(execution_id);
CREATE INDEX IF NOT EXISTS idx_execution_nodes_user ON execution_nodes(assigned_to_user_id, status);
CREATE INDEX IF NOT EXISTS idx_execution_nodes_role ON execution_nodes(assigned_to_role, status);

CREATE INDEX IF NOT EXISTS idx_carts_requester_status   ON access_carts(requester_id, status);
CREATE INDEX IF NOT EXISTS idx_cart_items_cart_id        ON access_cart_items(cart_id);
CREATE INDEX IF NOT EXISTS idx_cart_items_exec           ON access_cart_items(execution_id);
CREATE INDEX IF NOT EXISTS idx_executions_status         ON executions(status);
