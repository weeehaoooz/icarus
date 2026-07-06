-- SSE notification log: persisted so clients can replay missed events on reconnect.
-- The handler holds open connections per user_id in an in-memory broker;
-- this table is the durable fallback for reconnecting clients.
CREATE TABLE IF NOT EXISTS sse_notifications (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL,
    event_type TEXT NOT NULL, -- 'inbox.new' | 'cart.updated' | 'step.actioned'
    payload    TEXT NOT NULL, -- JSON blob
    is_read    BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sse_user_unread
    ON sse_notifications(user_id, is_read, created_at);
