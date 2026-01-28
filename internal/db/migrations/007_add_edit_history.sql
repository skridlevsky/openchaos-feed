-- Track comment edit history for transparency.
-- Each entry is {body, editedAt} recording the previous body before an edit.
ALTER TABLE events ADD COLUMN IF NOT EXISTS edit_history JSONB DEFAULT '[]';
