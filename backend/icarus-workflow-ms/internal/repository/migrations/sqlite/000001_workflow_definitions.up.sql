CREATE TABLE IF NOT EXISTS workflow_definitions (
    id             TEXT PRIMARY KEY,
    name           TEXT NOT NULL,
    definition_key TEXT NOT NULL,
    version        INTEGER NOT NULL DEFAULT 1,
    is_current     BOOLEAN NOT NULL DEFAULT 1,
    status         TEXT NOT NULL DEFAULT 'DRAFT', -- DRAFT | ACTIVE | SUPERSEDED | ARCHIVED
    supersedes_id  TEXT,
    metadata       TEXT, -- JSON blob: future use (e.g. sla config)
    created_by     TEXT NOT NULL,
    created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY(supersedes_id) REFERENCES workflow_definitions(id)
);

-- Only one definition per key can be current at a time
CREATE UNIQUE INDEX IF NOT EXISTS idx_workflow_def_current
    ON workflow_definitions(definition_key) WHERE is_current = 1;

CREATE INDEX IF NOT EXISTS idx_workflow_def_key ON workflow_definitions(definition_key);
