import { fetchStats, fetchEvents } from "@/lib/api";
import { StatsBar } from "@/components/StatsBar";
import { FeedList } from "@/components/FeedList";

export default async function HomePage() {
  let stats, feedData;
  try {
    [stats, feedData] = await Promise.all([
      fetchStats(),
      fetchEvents({ sort: "newest", limit: 50 }),
    ]);
  } catch {
    return (
      <div className="py-16 text-center text-zinc-500">
        <p className="text-lg">Unable to load feed data</p>
        <p className="text-sm mt-2">API may be unavailable</p>
      </div>
    );
  }

  return (
    <div>
      <h1 className="text-3xl font-bold mb-2">Activity Feed</h1>
      <p className="text-zinc-500 mb-8">
        Public record of governance activity for{" "}
        <a
          href="https://github.com/skridlevsky/openchaos"
          className="text-zinc-400 hover:text-emerald-400 transition-colors"
        >
          skridlevsky/openchaos
        </a>
      </p>
      {stats && <StatsBar stats={stats} />}
      <FeedList
        initialEvents={feedData?.events || []}
        initialHasMore={!!feedData?.nextCursor}
      />
    </div>
  );
}
