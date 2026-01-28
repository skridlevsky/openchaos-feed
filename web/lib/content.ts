interface EventContent {
  title?: string;
  body?: string;
}

export function extractContent(
  type: string,
  payload?: Record<string, unknown>
): EventContent {
  if (!payload) return {};

  switch (type) {
    // PR events - title + body from pull_request
    case "pr_opened":
    case "pr_closed":
    case "pr_merged":
    case "pr_reopened": {
      const pr = payload.pull_request as Record<string, unknown> | undefined;
      return { title: str(pr?.title), body: str(pr?.body) };
    }

    // Issue events - title + body from issue
    case "issue_opened":
    case "issue_closed":
    case "issue_reopened": {
      const issue = payload.issue as Record<string, unknown> | undefined;
      return { title: str(issue?.title), body: str(issue?.body) };
    }

    // Issue comment - issue title as context, comment body
    case "issue_comment": {
      const issue = payload.issue as Record<string, unknown> | undefined;
      const comment = payload.comment as Record<string, unknown> | undefined;
      return { title: str(issue?.title), body: str(comment?.body) };
    }

    // Review - PR title as context, review body
    case "review_submitted": {
      const pr = payload.pull_request as Record<string, unknown> | undefined;
      const review = payload.review as Record<string, unknown> | undefined;
      return { title: str(pr?.title), body: str(review?.body) };
    }

    // Review comment - PR title as context, comment body
    case "review_comment": {
      const pr = payload.pull_request as Record<string, unknown> | undefined;
      const comment = payload.comment as Record<string, unknown> | undefined;
      return { title: str(pr?.title), body: str(comment?.body) };
    }

    // Commit comment - short SHA as context, comment body
    case "commit_comment": {
      const comment = payload.comment as Record<string, unknown> | undefined;
      const commitId = str(comment?.commit_id);
      const title = commitId ? commitId.slice(0, 7) : undefined;
      return { title, body: str(comment?.body) };
    }

    // Discussion - handle both Events API and GraphQL shapes
    // Events API: {discussion: {title, body, ...}}
    // GraphQL:    {title, body, ...} (flat)
    case "discussion_created": {
      const disc =
        (payload.discussion as Record<string, unknown> | undefined) || payload;
      return { title: str(disc?.title), body: str(disc?.body) };
    }

    // Discussion comment - GraphQL shape: {body, ...} (flat)
    case "discussion_comment": {
      return { body: str(payload.body) };
    }

    // Push - branch name as title, commit messages with SHA links as body
    case "push": {
      const ref = str(payload.ref);
      const branch = ref?.replace(/^refs\/heads\//, "");
      const commits = payload.commits as
        | Array<Record<string, unknown>>
        | undefined;
      const messages = commits
        ?.map((c) => {
          const sha = str(c.sha);
          const msg = str(c.message);
          if (!msg) return null;
          const firstLine = msg.split("\n")[0];
          const shortSha = sha ? sha.slice(0, 7) : "";
          if (shortSha) {
            return `[\`${shortSha}\`](https://github.com/skridlevsky/openchaos/commit/${sha}) ${firstLine}`;
          }
          return firstLine;
        })
        .filter(Boolean) as string[];
      return {
        title: branch,
        body: messages?.length ? messages.join("\n") : undefined,
      };
    }

    // Release - name as title
    case "release": {
      const release = payload.release as Record<string, unknown> | undefined;
      return { title: str(release?.name) };
    }

    default:
      return {};
  }
}

function str(v: unknown): string | undefined {
  if (typeof v === "string" && v.trim()) return v.trim();
  return undefined;
}
