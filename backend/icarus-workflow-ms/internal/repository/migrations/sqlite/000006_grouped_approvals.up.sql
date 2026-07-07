ALTER TABLE workflow_stage_definitions ADD COLUMN type TEXT NOT NULL DEFAULT 'USER';
ALTER TABLE workflow_stage_definitions ADD COLUMN approval_tree TEXT;
ALTER TABLE workflow_stage_definitions DROP COLUMN execution_mode;
ALTER TABLE workflow_stage_definitions DROP COLUMN approval_quorum;

ALTER TABLE workflow_steps ADD COLUMN node_id TEXT;
CREATE INDEX IF NOT EXISTS idx_steps_node_id ON workflow_steps(node_id);
