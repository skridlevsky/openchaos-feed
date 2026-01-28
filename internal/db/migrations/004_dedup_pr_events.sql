-- Remove duplicate PR lifecycle events created by both backfill and Events API.
-- Keep the Events API version (payload contains "action" key) and delete the
-- backfill version (flat PR object without "action" key).
DELETE FROM events
WHERE id IN (
  SELECT e1.id
  FROM events e1
  JOIN events e2
    ON e1.pr_number = e2.pr_number
    AND e1.type = e2.type
    AND e1.github_user = e2.github_user
    AND e1.id != e2.id
  WHERE e1.type LIKE 'pr_%'
    AND e1.payload::text NOT LIKE '%"action"%'
    AND e2.payload::text LIKE '%"action"%'
);
