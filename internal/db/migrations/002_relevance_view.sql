-- 002_relevance_view.sql
-- Materialized view for relevance-sorted feed

CREATE MATERIALIZED VIEW IF NOT EXISTS feed_relevance AS
SELECT
    id,
    type,
    github_user,
    github_user_id,
    pr_number,
    issue_number,
    discussion_number,
    occurred_at,
    -- Relevance scoring algorithm
    -- relevance = (reaction_weight * 3) + (vote_weight * 2) + (1.0 / hours_since)
    COALESCE(
        -- Reaction count weight (estimate based on event type)
        (CASE WHEN type = 'reaction' THEN 3 ELSE 1 END) +
        -- Vote weight (reactions with choice field)
        (CASE WHEN choice IS NOT NULL THEN 2 ELSE 0 END) +
        -- Recency weight (1.0 / hours_since, capped at 24 hours)
        (1.0 / GREATEST(
            EXTRACT(EPOCH FROM (NOW() - occurred_at)) / 3600.0,
            1.0
        ))
    , 0) AS relevance_score
FROM events
WHERE occurred_at >= NOW() - INTERVAL '30 days' -- Only recent events for performance
ORDER BY relevance_score DESC, occurred_at DESC;

-- Index on the materialized view for fast queries
CREATE INDEX IF NOT EXISTS idx_feed_relevance_score ON feed_relevance(relevance_score DESC, occurred_at DESC);

-- Refresh the view (will be refreshed periodically in production)
REFRESH MATERIALIZED VIEW feed_relevance;
