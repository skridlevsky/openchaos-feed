-- Remove duplicate events that were inserted multiple times because their
-- github_id was NULL.  PostgreSQL treats NULL != NULL in unique constraints,
-- so ON CONFLICT (github_id) DO NOTHING never fired for push, star,
-- branch create/delete, wiki, member, and public events.
--
-- Strategy: for each group of rows sharing the same (type, github_user,
-- content_hash, occurred_at), keep the one with the smallest id (earliest
-- inserted) and delete the rest.
DELETE FROM events
WHERE id IN (
  SELECT id FROM (
    SELECT id,
           ROW_NUMBER() OVER (
             PARTITION BY type, github_user, content_hash, occurred_at
             ORDER BY id
           ) AS rn
    FROM events
    WHERE github_id IS NULL
  ) dupes
  WHERE rn > 1
);
