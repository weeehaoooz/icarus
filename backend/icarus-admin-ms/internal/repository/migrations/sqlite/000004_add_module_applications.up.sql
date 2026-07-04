CREATE TABLE IF NOT EXISTS module_applications (
    module_id TEXT NOT NULL,
    app_code TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (module_id, app_code)
);
