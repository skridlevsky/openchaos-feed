import type { FeedEvent } from "@/lib/types";
import { extractContent } from "@/lib/content";
import { ExpandableText } from "./ExpandableText";
import { EditHistory } from "./EditHistory";

const EVENT_CONFIG: Record<string, { icon: string; color: string; label: string }> = {
  pr_opened: { icon: "\u25D0", color: "text-blue-400", label: "PR opened" },
  pr_closed: { icon: "\u25D1", color: "text-purple-400", label: "PR closed" },
  pr_merged: { icon: "\u25CF", color: "text-emerald-400", label: "PR merged" },
  pr_reopened: { icon: "\u25D0", color: "text-blue-300", label: "PR reopened" },
  review_submitted: { icon: "\u2714", color: "text-green-400", label: "Review" },
  review_comment: { icon: "\u{1F4AC}", color: "text-zinc-400", label: "Review comment" },
  issue_opened: { icon: "\u25EF", color: "text-yellow-400", label: "Issue opened" },
  issue_closed: { icon: "\u25C9", color: "text-zinc-500", label: "Issue closed" },
  issue_comment: { icon: "\u25B8", color: "text-zinc-400", label: "Comment" },
  comment: { icon: "\u25B8", color: "text-zinc-400", label: "Comment" },
  reaction: { icon: "\u26A1", color: "text-amber-400", label: "Reaction" },
  star: { icon: "\u2605", color: "text-yellow-400", label: "Starred" },
  fork: { icon: "\u2442", color: "text-cyan-400", label: "Forked" },
  discussion_created: { icon: "\u25C8", color: "text-indigo-400", label: "Discussion" },
  discussion_comment: { icon: "\u25C7", color: "text-indigo-300", label: "Discussion comment" },
  discussion_answered: { icon: "\u2713", color: "text-green-400", label: "Discussion answered" },
};

const REACTION_EMOJI: Record<string, string> = {
  "+1": "\u{1F44D}",
  "-1": "\u{1F44E}",
  "laugh": "\u{1F604}",
  "confused": "\u{1F615}",
  "heart": "\u2764\uFE0F",
  "hooray": "\u{1F389}",
  "rocket": "\u{1F680}",
  "eyes": "\u{1F440}",
};

function getConfig(type: string) {
  return EVENT_CONFIG[type] || { icon: "\u00B7", color: "text-zinc-500", label: type };
}

function timeAgo(dateStr: string): string {
  const seconds = Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000);
  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return `${Math.floor(days / 30)}mo ago`;
}

const GITHUB_REPO = "https://github.com/skridlevsky/openchaos";

// Comment-type events show the parent PR/issue title inline instead of on a separate line
const COMMENT_TYPES = new Set([
  "issue_comment", "review_comment", "review_submitted",
  "commit_comment", "discussion_comment",
]);

export function EventCard({ event }: { event: FeedEvent }) {
  const config = getConfig(event.type);
  const content = extractContent(event.type, event.payload);
  const isComment = COMMENT_TYPES.has(event.type);
  const isPush = event.type === "push";

  const ref = event.prNumber
    ? { label: "PR", number: event.prNumber, href: `/pr/${event.prNumber}`, external: false }
    : event.issueNumber
      ? { label: "Issue", number: event.issueNumber, href: `${GITHUB_REPO}/issues/${event.issueNumber}`, external: true }
      : event.discussionNumber
        ? { label: "Discussion", number: event.discussionNumber, href: `${GITHUB_REPO}/discussions/${event.discussionNumber}`, external: true }
        : null;

  return (
    <article className="flex items-start gap-3 py-3 border-b border-zinc-800/50 last:border-0">
      <span className={`${config.color} text-lg mt-0.5 w-6 text-center shrink-0`}>
        {config.icon}
      </span>
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2 flex-wrap">
          <a
            href={`/voters/${event.githubUser}`}
            className="font-medium text-zinc-200 hover:text-emerald-400 transition-colors"
          >
            {event.githubUser}
          </a>
          <span className="text-zinc-500 text-sm">{config.label}</span>
          {ref && (
            <a
              href={ref.href}
              {...(ref.external ? { target: "_blank", rel: "noopener noreferrer" } : {})}
              className="text-sm text-zinc-400 hover:text-emerald-400 font-mono transition-colors"
            >
              #{ref.number}
            </a>
          )}
          {/* Comment events: show parent PR/issue title inline */}
          {isComment && content.title && (
            <span className="text-sm text-zinc-500 truncate min-w-0">
              {content.title}
            </span>
          )}
          {/* Push events: show branch name as code badge */}
          {isPush && content.title && (
            <code className="text-xs text-zinc-400 bg-zinc-800 px-1.5 py-0.5 rounded font-mono">
              {content.title}
            </code>
          )}
          {event.reactionType && (
            <span
              className={`text-sm px-1.5 py-0.5 rounded ${
                event.choice && event.choice > 0
                  ? "bg-emerald-500/10"
                  : event.choice && event.choice < 0
                    ? "bg-red-500/10"
                    : "bg-zinc-800"
              }`}
            >
              {REACTION_EMOJI[event.reactionType] || event.reactionType}
            </span>
          )}
        </div>
        {/* Regular events (PR, issue, discussion): title on its own line */}
        {!isComment && !isPush && content.title && (
          <p className="text-sm text-zinc-300 mt-1 truncate">
            {content.title}
          </p>
        )}
        {content.body && <ExpandableText text={content.body} />}
        {event.editHistory && event.editHistory.length > 0 && (
          <EditHistory entries={event.editHistory} />
        )}
        {event.reactionSummary && Object.keys(event.reactionSummary).length > 0 && (
          <div className="flex gap-1.5 mt-1.5">
            {Object.entries(event.reactionSummary)
              .sort(([, a], [, b]) => b - a)
              .map(([type, count]) => (
                <span
                  key={type}
                  className="text-xs text-zinc-500 bg-zinc-800/60 px-1.5 py-0.5 rounded-full"
                >
                  {REACTION_EMOJI[type] || type}{" "}
                  <span className="text-zinc-600">{count}</span>
                </span>
              ))}
          </div>
        )}
      </div>
      <div className="flex flex-col items-end gap-1 mt-1 shrink-0">
        <time
          dateTime={event.occurredAt}
          className="text-xs text-zinc-600 whitespace-nowrap font-mono"
        >
          {timeAgo(event.occurredAt)}
        </time>
        {event.editHistory && event.editHistory.length > 0 && (
          <span className="text-[10px] text-zinc-600 italic">edited</span>
        )}
      </div>
    </article>
  );
}
