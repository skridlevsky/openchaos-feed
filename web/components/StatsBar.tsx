import type { FeedStats } from "@/lib/types";

export function StatsBar({ stats }: { stats: FeedStats }) {
  const items = [
    { label: "Events", value: stats.totalEvents.toLocaleString() },
    { label: "Voters", value: stats.totalVoters.toLocaleString() },
    { label: "Votes", value: stats.totalVotes.toLocaleString() },
  ];

  return (
    <div className="grid grid-cols-3 gap-4 mb-8">
      {items.map((item) => (
        <div
          key={item.label}
          className="bg-zinc-900 rounded-lg px-4 py-3 border border-zinc-800"
        >
          <div className="text-2xl font-bold font-mono">{item.value}</div>
          <div className="text-sm text-zinc-500">{item.label}</div>
        </div>
      ))}
    </div>
  );
}
