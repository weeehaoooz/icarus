CREATE TABLE IF NOT EXISTS finance_assets (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    name          TEXT NOT NULL,
    ticker        TEXT NOT NULL DEFAULT '',
    isin          TEXT NOT NULL DEFAULT '',
    asset_class   TEXT NOT NULL,
    quantity      TEXT NOT NULL DEFAULT '0',
    unit_cost     TEXT NOT NULL DEFAULT '0',
    current_value TEXT NOT NULL DEFAULT '0',
    currency      TEXT NOT NULL DEFAULT 'SGD',
    exchange      TEXT NOT NULL DEFAULT '',
    country       TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    acquired_at   TEXT NOT NULL DEFAULT '',
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS finance_liabilities (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL,
    name            TEXT NOT NULL,
    liability_type  TEXT NOT NULL,
    principal       TEXT NOT NULL DEFAULT '0',
    outstanding_amt TEXT NOT NULL DEFAULT '0',
    interest_rate   TEXT NOT NULL DEFAULT '0',
    interest_type   TEXT NOT NULL DEFAULT 'fixed',
    currency        TEXT NOT NULL DEFAULT 'SGD',
    monthly_payment TEXT NOT NULL DEFAULT '0',
    term_months     INTEGER NOT NULL DEFAULT 0,
    maturity_date   TEXT NOT NULL DEFAULT '',
    lender          TEXT NOT NULL DEFAULT '',
    notes           TEXT NOT NULL DEFAULT '',
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS finance_insights (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL,
    category     TEXT NOT NULL,
    severity     TEXT NOT NULL,
    title        TEXT NOT NULL,
    body         TEXT NOT NULL,
    action_url   TEXT NOT NULL DEFAULT '',
    metadata     TEXT NOT NULL DEFAULT '{}',
    is_read      BOOLEAN NOT NULL DEFAULT 0,
    is_dismissed BOOLEAN NOT NULL DEFAULT 0,
    computed_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_finance_assets_user_id       ON finance_assets(user_id);
CREATE INDEX IF NOT EXISTS idx_finance_liabilities_user_id  ON finance_liabilities(user_id);
CREATE INDEX IF NOT EXISTS idx_finance_insights_user_id     ON finance_insights(user_id);
