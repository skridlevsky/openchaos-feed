import { fetchPRVotes, fetchPREvents } from "@/lib/api";
import { EventCard } from "@/components/EventCard";
import { VoteBreakdown } from "@/components/VoteBreakdown";
import { CollapsibleSection } from "@/components/CollapsibleSection";
import type { Metadata } from "next";

const GITHUB_REPO = "https://github.com/skridlevsky/openchaos";

type Props = { params: Promise<{ number: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { number } = await params;
  try {
    const data = await fetchPRVotes(Number(number));
    return {
      title: `PR #${number} — Vote Breakdown`,
      description: `${data.upvotes} up, ${data.downvotes} down — ${data.upvotes + data.downvotes} total votes on PR #${number}`,
    };
  } catch {
    return { title: `PR #${number} — Vote Breakdown` };
  }
}

export default async function PRDetailPage({ params }: Props) {
  const { number } = await params;
  const prNumber = Number(number);

  let data, events;
  try {
    [data, events] = await Promise.all([
      fetchPRVotes(prNumber),
      fetchPREvents(prNumber).catch(() => []),
    ]);
  } catch {
    return (
      <div className="py-16 text-center text-zinc-500">
        No vote data found for PR #{number}
      </div>
    );
  }

  const comments = events.filter(
    (e) =>
      e.type === "issue_comment" ||
      e.type === "review_submitted" ||
      e.type === "review_comment" ||
      e.type === "commit_comment"
  );

  return (
    <div>
      <div className="mb-8 flex flex-col sm:flex-row sm:items-center gap-4">
        <div className="flex-1">
          <h1 className="text-3xl font-bold mb-1">PR #{prNumber}</h1>
          <p className="text-sm text-zinc-500">
            {data.upvotes + data.downvotes} votes &middot;{" "}
            {data.upvotes} up &middot; {data.downvotes} down
          </p>
        </div>
        <a
          href={`${GITHUB_REPO}/pull/${prNumber}`}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-zinc-800 border border-zinc-700 text-zinc-200 hover:bg-zinc-700 hover:border-zinc-600 transition-colors text-sm font-medium shrink-0"
        >
          <svg
            viewBox="0 0 16 16"
            width="16"
            height="16"
            fill="currentColor"
            className="opacity-70"
          >
            <path d="M8 0c4.42 0 8 3.58 8 8a8.013 8.013 0 0 1-5.45 7.59c-.4.08-.55-.17-.55-.38 0-.27.01-1.13.01-2.2 0-.75-.25-1.23-.54-1.48 1.78-.2 3.65-.88 3.65-3.95 0-.88-.31-1.59-.82-2.15.08-.2.36-1.02-.08-2.12 0 0-.67-.22-2.2.82-.64-.18-1.32-.27-2-.27-.68 0-1.36.09-2 .27-1.53-1.03-2.2-.82-2.2-.82-.44 1.1-.16 1.92-.08 2.12-.51.56-.82 1.28-.82 2.15 0 3.06 1.86 3.75 3.64 3.95-.23.2-.44.55-.51 1.07-.46.21-1.61.55-2.33-.66-.15-.24-.6-.83-1.23-.82-.67.01-.27.38.01.53.34.19.73.9.82 1.13.16.45.68 1.31 2.69.94 0 .67.01 1.3.01 1.49 0 .21-.15.45-.55.38A7.995 7.995 0 0 1 0 8c0-4.42 3.58-8 8-8Z" />
          </svg>
          View on GitHub
        </a>
      </div>

      <div className="grid grid-cols-3 gap-4 mb-8">
        <div className="bg-zinc-900 rounded-lg px-4 py-3 border border-zinc-800">
          <div className="text-2xl font-bold font-mono">
            {data.upvotes + data.downvotes}
          </div>
          <div className="text-sm text-zinc-500">Total Votes</div>
        </div>
        <div className="bg-zinc-900 rounded-lg px-4 py-3 border border-zinc-800">
          <div className="text-2xl font-bold font-mono text-emerald-400">
            +{data.upvotes}
          </div>
          <div className="text-sm text-zinc-500">Upvotes</div>
        </div>
        <div className="bg-zinc-900 rounded-lg px-4 py-3 border border-zinc-800">
          <div className="text-2xl font-bold font-mono text-red-400">
            -{data.downvotes}
          </div>
          <div className="text-sm text-zinc-500">Downvotes</div>
        </div>
      </div>

      <div className="mb-8 max-w-lg">
        <VoteBreakdown upvotes={data.upvotes} downvotes={data.downvotes} />
      </div>

      {comments.length > 0 && (
        <div className="mb-8">
          <h2 className="text-xl font-bold mb-4">
            Comments
            <span className="text-sm font-normal text-zinc-500 ml-2">
              ({comments.length})
            </span>
          </h2>
          <div>
            {comments.map((event) => (
              <EventCard key={event.id} event={event} />
            ))}
          </div>
        </div>
      )}

      <CollapsibleSection
        title="Voters"
        count={data.voters.length}
        defaultOpen={comments.length === 0}
      >
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-zinc-800 text-left text-zinc-500">
                <th className="pb-3 pr-4 font-medium">Voter</th>
                <th className="pb-3 pr-4 font-medium">Vote</th>
                <th className="pb-3 font-medium text-right">Time</th>
              </tr>
            </thead>
            <tbody>
              {data.voters.map((voter) => (
                <tr
                  key={`${voter.githubUser}-${voter.votedAt}`}
                  className="border-b border-zinc-800/50 hover:bg-zinc-900/50 transition-colors"
                >
                  <td className="py-3 pr-4">
                    <a
                      href={`/voters/${voter.githubUser}`}
                      className="text-zinc-200 hover:text-emerald-400 transition-colors font-medium"
                    >
                      {voter.githubUser}
                    </a>
                  </td>
                  <td className="py-3 pr-4">
                    <span
                      className={`font-mono text-sm px-2 py-0.5 rounded ${
                        voter.choice > 0
                          ? "bg-emerald-500/10 text-emerald-400"
                          : "bg-red-500/10 text-red-400"
                      }`}
                    >
                      {voter.choice > 0 ? "+1" : "-1"}
                    </span>
                  </td>
                  <td className="py-3 text-right text-zinc-500">
                    <time dateTime={voter.votedAt}>
                      {new Date(voter.votedAt).toLocaleDateString()}
                    </time>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </CollapsibleSection>

      {events.length > 0 && events.length !== comments.length && (
        <div className="mt-8">
          <h2 className="text-xl font-bold mb-4">
            All Activity
            <span className="text-sm font-normal text-zinc-500 ml-2">
              ({events.length})
            </span>
          </h2>
          <div>
            {events.map((event) => (
              <EventCard key={event.id} event={event} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
