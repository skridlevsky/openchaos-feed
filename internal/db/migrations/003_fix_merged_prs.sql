-- Fix backfill data: reclassify pr_closed events that were actually merged.
-- These 13 PRs have merged_at set on GitHub but the backfill stored them
-- as pr_closed with merged: false in the flat payload.
UPDATE events
SET type = 'pr_merged'
WHERE type = 'pr_closed'
  AND pr_number IN (6, 8, 11, 14, 47, 51, 52, 60, 63, 71, 73, 86, 119);
