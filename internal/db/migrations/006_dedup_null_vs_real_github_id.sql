-- Remove legacy NULL-github_id rows that now have a proper duplicate
-- with a real github_id (ingested after the dedup fix was deployed).
-- Match on (type, github_user, content_hash, occurred_at).
DELETE FROM events
WHERE id IN (
  SELECT e_null.id
  FROM events e_null
  JOIN events e_real
    ON e_null.type = e_real.type
    AND e_null.github_user = e_real.github_user
    AND e_null.content_hash = e_real.content_hash
    AND e_null.occurred_at = e_real.occurred_at
  WHERE e_null.github_id IS NULL
    AND e_real.github_id IS NOT NULL
);
