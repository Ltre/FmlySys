CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS data_partitions (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    path TEXT NOT NULL UNIQUE,
    source_type TEXT NOT NULL DEFAULT 'INITIAL',
    created_at TEXT NOT NULL,
    last_opened_at TEXT,
    is_active INTEGER NOT NULL DEFAULT 0 CHECK (is_active IN (0,1))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_one_active_partition
ON data_partitions(is_active) WHERE is_active = 1;
