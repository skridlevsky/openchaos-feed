import { fetchVoters } from "@/lib/api";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Voters — Governance Leaderboard",
  description:
    "All voters who participated in OpenChaos governance. Ranked by total votes cast.",
};

export default async function VotersPage() {
  let voters;
  try {
    voters = await fetchVoters();
  } catch {
    return (
      <div className="py-16 text-center text-zinc-500">
        Unable to load voter data
      </div>
    );
  }

  return (
    <div>
      <h1 className="text-3xl font-bold mb-2">Voter Leaderboard</h1>
      <p className="text-zinc-500 mb-8">
        {voters.length.toLocaleString()} voters have participated in OpenChaos governance
      </p>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-zinc-800 text-left text-zinc-500">
              <th className="pb-3 pr-4 font-medium">#</th>
              <th className="pb-3 pr-4 font-medium">Voter</th>
              <th className="pb-3 pr-4 font-medium text-right">Votes</th>
              <th className="pb-3 pr-4 font-medium text-right">
                <span className="text-emerald-500">+</span>
              </th>
              <th className="pb-3 pr-4 font-medium text-right">
                <span className="text-red-500">-</span>
              </th>
              <th className="pb-3 pr-4 font-medium text-right">PRs</th>
              <th className="pb-3 font-medium text-right">Last Vote</th>
            </tr>
          </thead>
          <tbody>
            {voters.map((voter, i) => (
              <tr
                key={voter.githubUser}
                className="border-b border-zinc-800/50 hover:bg-zinc-900/50 transition-colors"
              >
                <td className="py-3 pr-4 text-zinc-600 font-mono">{i + 1}</td>
                <td className="py-3 pr-4">
                  <a
                    href={`/voters/${voter.githubUser}`}
                    className="text-zinc-200 hover:text-emerald-400 transition-colors font-medium"
                  >
                    {voter.githubUser}
                  </a>
                </td>
                <td className="py-3 pr-4 text-right font-mono">{voter.totalVotes}</td>
                <td className="py-3 pr-4 text-right font-mono text-emerald-400">
                  {voter.upvotes}
                </td>
                <td className="py-3 pr-4 text-right font-mono text-red-400">
                  {voter.downvotes}
                </td>
                <td className="py-3 pr-4 text-right font-mono text-zinc-400">
                  {voter.uniquePrs}
                </td>
                <td className="py-3 text-right text-zinc-500">
                  <time dateTime={voter.lastVote}>
                    {new Date(voter.lastVote).toLocaleDateString()}
                  </time>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
