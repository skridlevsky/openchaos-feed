import { fetchVoter, fetchUserEvents } from "@/lib/api";
import { EventCard } from "@/components/EventCard";
import { VoteBreakdown } from "@/components/VoteBreakdown";
import type { Metadata } from "next";

type Props = { params: Promise<{ username: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { username } = await params;
  return {
    title: `${username} — Voter Profile`,
    description: `Governance voting history for ${username} on OpenChaos.`,
  };
}

export default async function VoterDetailPage({ params }: Props) {
  const { username } = await params;

  let voter, userEvents;
  try {
    [voter, userEvents] = await Promise.all([
      fetchVoter(username),
      fetchUserEvents(username),
    ]);
  } catch {
    return (
      <div className="py-16 text-center text-zinc-500">
        Voter &ldquo;{username}&rdquo; not found
      </div>
    );
  }

  return (
    <div>
      <div className="mb-8">
        <h1 className="text-3xl font-bold mb-1">{username}</h1>
        <a
          href={`https://github.com/${username}`}
          target="_blank"
          rel="noopener noreferrer"
          className="text-zinc-500 hover:text-zinc-300 text-sm transition-colors"
        >
          github.com/{username}
        </a>
      </div>

      <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mb-8">
        <div className="bg-zinc-900 rounded-lg px-4 py-3 border border-zinc-800">
          <div className="text-2xl font-bold font-mono">{voter.totalVotes}</div>
          <div className="text-sm text-zinc-500">Total Votes</div>
        </div>
        <div className="bg-zinc-900 rounded-lg px-4 py-3 border border-zinc-800">
          <div className="text-2xl font-bold font-mono text-emerald-400">
            {voter.upvotes}
          </div>
          <div className="text-sm text-zinc-500">Upvotes</div>
        </div>
        <div className="bg-zinc-900 rounded-lg px-4 py-3 border border-zinc-800">
          <div className="text-2xl font-bold font-mono text-red-400">
            {voter.downvotes}
          </div>
          <div className="text-sm text-zinc-500">Downvotes</div>
        </div>
        <div className="bg-zinc-900 rounded-lg px-4 py-3 border border-zinc-800">
          <div className="text-2xl font-bold font-mono">{voter.uniquePrs}</div>
          <div className="text-sm text-zinc-500">Unique PRs</div>
        </div>
      </div>

      {(voter.upvotes > 0 || voter.downvotes > 0) && (
        <div className="mb-8 max-w-md">
          <VoteBreakdown upvotes={voter.upvotes} downvotes={voter.downvotes} />
        </div>
      )}

      <h2 className="text-xl font-bold mb-4">Activity</h2>
      <div>
        {userEvents.map((event) => (
          <EventCard key={event.id} event={event} />
        ))}
        {userEvents.length === 0 && (
          <p className="text-zinc-500 py-8 text-center">No activity found</p>
        )}
      </div>
    </div>
  );
}
