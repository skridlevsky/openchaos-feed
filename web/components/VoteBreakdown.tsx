export function VoteBreakdown({
  upvotes,
  downvotes,
}: {
  upvotes: number;
  downvotes: number;
}) {
  const total = upvotes + downvotes;
  if (total === 0) return null;
  const upPct = (upvotes / total) * 100;

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-sm font-mono">
        <span className="text-emerald-400">+{upvotes}</span>
        <span className="text-red-400">-{downvotes}</span>
      </div>
      <div className="h-2 rounded-full bg-zinc-800 overflow-hidden flex">
        <div
          className="bg-emerald-500 transition-all"
          style={{ width: `${upPct}%` }}
        />
        <div className="bg-red-500 flex-1" />
      </div>
    </div>
  );
}
