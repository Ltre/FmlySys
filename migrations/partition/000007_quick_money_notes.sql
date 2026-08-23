CREATE TABLE IF NOT EXISTS quick_money_notes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL CHECK (category IN ('expense','transfer','reimbursement','asset_event')),
    summary TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','standardized')),
    standardized_entity_type TEXT,
    standardized_entity_id INTEGER,
    created_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL,
    standardized_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_quick_money_notes_creator_status
ON quick_money_notes(created_by, status, id);
