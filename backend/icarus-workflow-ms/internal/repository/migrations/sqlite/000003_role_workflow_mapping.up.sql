CREATE TABLE IF NOT EXISTS role_workflow_mappings (
    id                     TEXT PRIMARY KEY,
    role_id                TEXT NOT NULL UNIQUE, -- one active mapping per role
    workflow_definition_id TEXT NOT NULL,
    is_active              BOOLEAN NOT NULL DEFAULT 1,
    effective_from         DATETIME DEFAULT CURRENT_TIMESTAMP,
    effective_to           DATETIME,
    created_by             TEXT NOT NULL,
    created_at             DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(workflow_definition_id)
        REFERENCES workflow_definitions(id)
);

CREATE INDEX IF NOT EXISTS idx_role_workflow_mapping_role
    ON role_workflow_mappings(role_id, is_active);
