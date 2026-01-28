"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { EventCard } from "./EventCard";
import type { FeedEvent } from "@/lib/types";

const FILTER_STORAGE_KEY = "openchaos-feed-filter";
const SORT_STORAGE_KEY = "openchaos-feed-sort";

const EVENT_FILTERS = [
  { label: "All", value: "" },
  { label: "Votes", value: "reaction" },
  { label: "Comments", value: "issue_comment,review_comment,review_submitted,commit_comment,discussion_comment" },
  { label: "PRs", value: "pr_opened,pr_closed,pr_merged,pr_reopened" },
  { label: "Stars", value: "star" },
  { label: "Discussions", value: "discussion_created,discussion_answered" },
];

const SORT_OPTIONS = [
  { label: "Newest", value: "newest" },
  { label: "Oldest", value: "oldest" },
];

const API_URL = process.env.NEXT_PUBLIC_API_URL || "";

function getSaved(key: string, fallback: string): string {
  if (typeof window === "undefined") return fallback;
  try {
    return localStorage.getItem(key) || fallback;
  } catch {
    return fallback;
  }
}

function save(key: string, value: string) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // localStorage unavailable
  }
}

export function FeedList({
  initialEvents,
  initialHasMore,
}: {
  initialEvents: FeedEvent[];
  initialHasMore: boolean;
}) {
  const [events, setEvents] = useState(initialEvents);
  const [hasMore, setHasMore] = useState(initialHasMore);
  const [cursor, setCursor] = useState<string | null>(
    initialEvents.length > 0 ? initialEvents[initialEvents.length - 1].id : null
  );
  const [filter, setFilter] = useState("");
  const [sort, setSort] = useState("newest");
  const [loading, setLoading] = useState(false);
  const [restored, setRestored] = useState(false);

  // Restore saved filter + sort on mount
  useEffect(() => {
    const savedFilter = getSaved(FILTER_STORAGE_KEY, "");
    const savedSort = getSaved(SORT_STORAGE_KEY, "newest");

    const needsRefetch = savedFilter !== "" || savedSort !== "newest";
    if (needsRefetch) {
      refetch(savedFilter, savedSort);
    }
    setFilter(savedFilter);
    setSort(savedSort);
    setRestored(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Fetch a fresh page with given filter + sort
  const refetch = async (filterVal: string, sortVal: string) => {
    setLoading(true);
    try {
      const params = new URLSearchParams({ sort: sortVal, limit: "50" });
      if (filterVal) params.set("type", filterVal);
      const res = await fetch(`${API_URL}/api/feed/?${params}`);
      const data = await res.json();
      setEvents(data.events || []);
      setHasMore(!!data.nextCursor);
      setCursor(data.nextCursor || null);
    } catch (err) {
      console.error("Failed to fetch events:", err);
    } finally {
      setLoading(false);
    }
  };

  const loadMore = useCallback(async () => {
    if (loading || !hasMore || !cursor) return;
    setLoading(true);
    try {
      const params = new URLSearchParams({ sort, limit: "50", cursor });
      if (filter) params.set("type", filter);
      const res = await fetch(`${API_URL}/api/feed/?${params}`);
      const data = await res.json();
      setEvents((prev) => {
        const seen = new Set(prev.map((e) => e.id));
        const next = (data.events || []).filter((e: FeedEvent) => !seen.has(e.id));
        return [...prev, ...next];
      });
      setHasMore(!!data.nextCursor);
      setCursor(data.nextCursor || null);
    } catch (err) {
      console.error("Failed to load more events:", err);
    } finally {
      setLoading(false);
    }
  }, [loading, hasMore, cursor, filter, sort]);

  const applyFilter = (value: string) => {
    setFilter(value);
    save(FILTER_STORAGE_KEY, value);
    refetch(value, sort);
  };

  const applySort = (value: string) => {
    setSort(value);
    save(SORT_STORAGE_KEY, value);
    refetch(filter, value);
  };

  // Throttled scroll handler — fires at most once per 200ms
  const scrollThrottleRef = useRef(false);
  useEffect(() => {
    const handleScroll = () => {
      if (scrollThrottleRef.current) return;
      scrollThrottleRef.current = true;
      requestAnimationFrame(() => {
        if (
          window.innerHeight + document.documentElement.scrollTop >=
          document.documentElement.offsetHeight - 500
        ) {
          loadMore();
        }
        setTimeout(() => {
          scrollThrottleRef.current = false;
        }, 200);
      });
    };
    window.addEventListener("scroll", handleScroll, { passive: true });
    return () => window.removeEventListener("scroll", handleScroll);
  }, [loadMore]);

  return (
    <div>
      {/* Toolbar: filters + sort */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-6">
        {/* Filters */}
        <div className="flex gap-2 flex-wrap">
          {EVENT_FILTERS.map((f) => (
            <button
              key={f.value}
              onClick={() => applyFilter(f.value)}
              className={`px-3 py-1.5 rounded-full text-sm transition-colors ${
                filter === f.value
                  ? "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30"
                  : "bg-zinc-800 text-zinc-400 border border-zinc-700 hover:border-zinc-600"
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>
        {/* Sort */}
        <div className="flex items-center gap-1.5 shrink-0">
          <span className="text-xs text-zinc-600 mr-1">Sort</span>
          {SORT_OPTIONS.map((s) => (
            <button
              key={s.value}
              onClick={() => applySort(s.value)}
              className={`px-2.5 py-1 rounded text-xs font-medium transition-colors ${
                sort === s.value
                  ? "bg-zinc-700 text-zinc-200"
                  : "text-zinc-500 hover:text-zinc-300"
              }`}
            >
              {s.label}
            </button>
          ))}
        </div>
      </div>

      <div className={restored ? "" : "opacity-0"}>
        {events.map((event) => (
          <EventCard key={event.id} event={event} />
        ))}
      </div>
      {loading && (
        <div className="py-8 text-center text-zinc-500">Loading...</div>
      )}
      {!hasMore && events.length > 0 && (
        <div className="py-8 text-center text-zinc-600 text-sm">
End of feed
        </div>
      )}
      {events.length === 0 && !loading && (
        <div className="py-16 text-center text-zinc-500">No events found</div>
      )}
    </div>
  );
}
