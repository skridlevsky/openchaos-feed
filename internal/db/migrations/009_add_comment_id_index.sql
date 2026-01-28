-- Add index on comment_id for UpdateCommentEdit and DeleteByCommentID operations.
-- Without this, those queries perform sequential scans on the events table.
CREATE INDEX IF NOT EXISTS idx_events_comment_id
    ON events(comment_id)
    WHERE comment_id IS NOT NULL;
