export interface EditHistoryEntry {
  body: string;
  editedAt: string;
}

export interface FeedEvent {
  id: string;
  type: string;
  githubUser: string;
  githubUserId: number;
  prNumber?: number;
  issueNumber?: number;
  discussionNumber?: number;
  commentId?: number;
  choice?: number;
  reactionType?: string;
  githubId?: number;
  payload?: Record<string, unknown>;
  contentHash: string;
  editHistory?: EditHistoryEntry[];
  occurredAt: string;
  ingestedAt: string;
}

export interface FeedStats {
  totalEvents: number;
  totalVotes: number;
  totalVoters: number;
  latestEventAt?: string;
  eventsByType: Record<string, number>;
  eventsLastHour: number;
}

export interface ListResponse {
  events: FeedEvent[];
  nextCursor?: string;
  totalCount: number;
}

export interface VoterSummary {
  githubUser: string;
  githubUserId: number;
  totalVotes: number;
  upvotes: number;
  downvotes: number;
  firstVote: string;
  lastVote: string;
  prsVotedOn: number[];
  uniquePrs: number;
}

export interface PRVotesResponse {
  prNumber: number;
  upvotes: number;
  downvotes: number;
  net: number;
  voters: VoterVoteDetail[];
}

export interface VoterVoteDetail {
  githubUser: string;
  choice: number;
  votedAt: string;
}
