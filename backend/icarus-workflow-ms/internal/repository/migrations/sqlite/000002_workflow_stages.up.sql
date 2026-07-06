CREATE TABLE IF NOT EXISTS workflow_stage_definitions (
    id                     TEXT PRIMARY KEY,
    workflow_definition_id TEXT NOT NULL,
    sequence_order         INTEGER NOT NULL,
    execution_mode         TEXT NOT NULL DEFAULT 'SEQUENTIAL', -- SEQUENTIAL | PARALLEL
    name                   TEXT NOT NULL,
    approval_quorum        INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY(workflow_definition_id)
        REFERENCES workflow_definitions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS approver_definitions (
    id                  TEXT PRIMARY KEY,
    stage_definition_id TEXT NOT NULL,
    resolver_type       TEXT NOT NULL, -- USER | ROLE_QUEUE | EXPRESSION
    resolver_value      TEXT,          -- user_id | role_name | expression string
    FOREIGN KEY(stage_definition_id)
        REFERENCES workflow_stage_definitions(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_stage_defs_workflow_id
    ON workflow_stage_definitions(workflow_definition_id);

CREATE INDEX IF NOT EXISTS idx_approver_defs_stage_id
    ON approver_definitions(stage_definition_id);
