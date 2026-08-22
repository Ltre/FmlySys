ALTER TABLE members
ADD COLUMN is_del INTEGER NOT NULL DEFAULT 0 CHECK (is_del IN (0,1));

CREATE INDEX IF NOT EXISTS idx_members_status_delete
ON members(status, is_del, id);
