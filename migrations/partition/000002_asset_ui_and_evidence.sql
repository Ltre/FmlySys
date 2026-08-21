-- Normalize historical duplicate INITIAL_ASSET rows before enforcing one active initial record per member.
UPDATE asset_events
SET event_type = 'ASSET_IN'
WHERE event_type = 'INITIAL_ASSET'
  AND status = 'active'
  AND id NOT IN (
      SELECT MIN(id)
      FROM asset_events
      WHERE event_type = 'INITIAL_ASSET' AND status = 'active'
      GROUP BY holder_member_id
  );

ALTER TABLE asset_events ADD COLUMN related_event_id INTEGER REFERENCES asset_events(id);

ALTER TABLE public_expenses ADD COLUMN public_paid_amount_cent INTEGER NOT NULL DEFAULT 0 CHECK (public_paid_amount_cent >= 0);

UPDATE public_expenses
SET public_paid_amount_cent = CASE WHEN funding_type = 'PUBLIC_HELD_ASSET' THEN amount_cent ELSE 0 END;

CREATE UNIQUE INDEX IF NOT EXISTS idx_asset_events_one_initial_per_holder
ON asset_events(holder_member_id)
WHERE event_type = 'INITIAL_ASSET' AND status = 'active';

CREATE INDEX IF NOT EXISTS idx_asset_events_related
ON asset_events(related_event_id);

CREATE TABLE IF NOT EXISTS record_attachments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    entity_type TEXT NOT NULL,
    entity_id INTEGER NOT NULL,
    storage_name TEXT NOT NULL UNIQUE,
    original_name TEXT NOT NULL,
    mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    size INTEGER NOT NULL CHECK (size > 0 AND size <= 10485760),
    sha256 TEXT NOT NULL,
    uploaded_by INTEGER NOT NULL REFERENCES members(id),
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_record_attachments_entity
ON record_attachments(entity_type, entity_id, id);
