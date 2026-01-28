-- 001_create_events.sql
-- Events table for GitHub activity feed

CREATE TABLE IF NOT EXISTS events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type VARCHAR(50) NOT NULL,
    github_user VARCHAR(100) NOT NULL,
    github_user_id BIGINT NOT NULL,
    pr_number INT,
    issue_number INT,
    discussion_number INT,
    comment_id BIGINT,
    choice SMALLINT, -- +1 or -1 for votes (reactions)
    reaction_type VARCHAR(20),
    github_id BIGINT,
    payload JSONB NOT NULL,
    content_hash VARCHAR(66) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT unique_github_id UNIQUE (github_id)
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_events_github_user ON events(github_user);
CREATE INDEX IF NOT EXISTS idx_events_github_user_id ON events(github_user_id);
CREATE INDEX IF NOT EXISTS idx_events_pr_number ON events(pr_number) WHERE pr_number IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_issue_number ON events(issue_number) WHERE issue_number IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_discussion_number ON events(discussion_number) WHERE discussion_number IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_events_occurred_at ON events(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_choice ON events(choice) WHERE choice IS NOT NULL;

-- Composite index for voting queries
CREATE INDEX IF NOT EXISTS idx_events_votes ON events(type, choice, pr_number)
    WHERE type = 'reaction' AND choice IS NOT NULL;

-- Index for voter aggregation queries
CREATE INDEX IF NOT EXISTS idx_events_voter_stats ON events(github_user, type, choice)
    WHERE type = 'reaction';
