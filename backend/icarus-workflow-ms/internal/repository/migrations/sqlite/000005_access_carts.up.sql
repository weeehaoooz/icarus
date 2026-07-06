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

CREATE TABLE IF NOT EXISTS access_cart_items (
    id                   TEXT PRIMARY KEY,
    cart_id              TEXT NOT NULL,
    role_id              TEXT NOT NULL,
    role_name            TEXT NOT NULL,
    status               TEXT NOT NULL DEFAULT 'PENDING', -- PENDING | IN_PROGRESS | APPROVED | REJECTED | CANCELLED
    workflow_instance_id TEXT,
    created_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(cart_id) REFERENCES access_carts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS workflow_instances (
    id                     TEXT PRIMARY KEY,
    workflow_definition_id TEXT NOT NULL,
    cart_item_id           TEXT NOT NULL UNIQUE,
    status                 TEXT NOT NULL DEFAULT 'IN_PROGRESS', -- IN_PROGRESS | COMPLETED | REJECTED | CANCELLED
    current_stage_seq      INTEGER NOT NULL DEFAULT 1,
    started_at             DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at           DATETIME,
    version                INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(workflow_definition_id) REFERENCES workflow_definitions(id),
    FOREIGN KEY(cart_item_id) REFERENCES access_cart_items(id)
);

CREATE TABLE IF NOT EXISTS workflow_steps (
    id                   TEXT PRIMARY KEY,
    workflow_instance_id TEXT NOT NULL,
    stage_definition_id  TEXT NOT NULL,
    assigned_to_user_id  TEXT, -- NULL if role-queue assignment
    assigned_to_role     TEXT, -- NULL if user assignment
    status               TEXT NOT NULL DEFAULT 'PENDING', -- PENDING | APPROVED | REJECTED | DELEGATED
    decision_comment     TEXT,
    acted_by_user_id     TEXT,
    assigned_at          DATETIME DEFAULT CURRENT_TIMESTAMP,
    acted_at             DATETIME,
    version              INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(workflow_instance_id) REFERENCES workflow_instances(id),
    FOREIGN KEY(stage_definition_id)  REFERENCES workflow_stage_definitions(id)
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id             TEXT PRIMARY KEY,
    entity_type    TEXT NOT NULL, -- CART | CART_ITEM | WORKFLOW_INSTANCE | WORKFLOW_STEP
    entity_id      TEXT NOT NULL,
    actor_user_id  TEXT,
    action         TEXT NOT NULL, -- SUBMITTED | APPROVED | REJECTED | DELEGATED | AUTO_APPROVED | ...
    before_state   TEXT,          -- JSON blob
    after_state    TEXT,          -- JSON blob
    correlation_id TEXT,
    occurred_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_carts_requester_status   ON access_carts(requester_id, status);
CREATE INDEX IF NOT EXISTS idx_cart_items_cart_id        ON access_cart_items(cart_id);
CREATE INDEX IF NOT EXISTS idx_cart_items_workflow_inst  ON access_cart_items(workflow_instance_id);
CREATE INDEX IF NOT EXISTS idx_instances_status          ON workflow_instances(status);
CREATE INDEX IF NOT EXISTS idx_steps_user_status         ON workflow_steps(assigned_to_user_id, status);
CREATE INDEX IF NOT EXISTS idx_steps_role_status         ON workflow_steps(assigned_to_role, status);
CREATE INDEX IF NOT EXISTS idx_steps_instance_id         ON workflow_steps(workflow_instance_id);
CREATE INDEX IF NOT EXISTS idx_audit_entity              ON audit_logs(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_correlation         ON audit_logs(correlation_id);
