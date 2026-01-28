import type { FeedEvent, FeedStats, ListResponse, VoterSummary, PRVotesResponse } from "./types";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

async function fetchAPI<T>(path: string, revalidate = 60): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    next: { revalidate },
  });
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`);
  }
  return res.json();
}

export async function fetchStats() {
  return fetchAPI<FeedStats>("/api/feed/stats");
}

export async function fetchEvents(params?: {
  sort?: string;
  limit?: number;
  type?: string;
  cursor?: string;
}) {
  const searchParams = new URLSearchParams();
  if (params?.sort) searchParams.set("sort", params.sort);
  if (params?.limit) searchParams.set("limit", String(params.limit));
  if (params?.type) searchParams.set("type", params.type);
  if (params?.cursor) searchParams.set("cursor", params.cursor);
  const query = searchParams.toString();
  return fetchAPI<ListResponse>(`/api/feed/${query ? `?${query}` : ""}`);
}

export async function fetchVoters() {
  return fetchAPI<VoterSummary[]>("/api/feed/voters");
}

export async function fetchVoter(username: string) {
  return fetchAPI<VoterSummary>(`/api/feed/voters/${username}`);
}

export async function fetchUserEvents(username: string) {
  return fetchAPI<FeedEvent[]>(`/api/feed/user/${username}`);
}

export async function fetchPRVotes(number: number) {
  return fetchAPI<PRVotesResponse>(`/api/feed/votes/pr/${number}`);
}

export async function fetchPREvents(number: number) {
  return fetchAPI<FeedEvent[]>(`/api/feed/pr/${number}`);
}
