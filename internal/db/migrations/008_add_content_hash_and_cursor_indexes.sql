-- 008_add_content_hash_and_cursor_indexes.sql
-- content_hash index: speeds up INSERT dedup WHERE NOT EXISTS check
CREATE INDEX IF NOT EXISTS idx_events_content_hash
    ON events(content_hash);

-- Composite index for cursor pagination with tiebreaker
-- Ensures consistent ordering when multiple events share the same occurred_at
CREATE INDEX IF NOT EXISTS idx_events_occurred_at_id
    ON events(occurred_at DESC, id DESC);
