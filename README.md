# openchaos-feed

Public governance activity feed for [OpenChaos](https://github.com/skridlevsky/openchaos). Ingests GitHub events (PRs, issues, comments, reactions, discussions, stars, forks) into Postgres and serves them via REST API + SSR frontend.

Built for transparency and research — governance data should be searchable public record.

Research: Voting data for [TU Delft trust & governance research (MeritRank)](https://github.com/Tribler/tribler/issues/8667)

## Architecture

```
feed.openchaos.dev → Next.js SSR (web/)
api-feed.openchaos.dev → Go API (cmd/server)
```

### Go API

- 3 polling ingesters: Events API (60s), Reactions (5min), Discussions GraphQL (10min)
- Postgres storage with cursor-based pagination
- Voter leaderboard + PR vote breakdown endpoints

### Next.js Frontend

- Server-side rendered for SEO (governance data = public record)
- Dynamic metadata for PR and voter pages
- Dark theme, infinite scroll activity feed

## API Endpoints

```
GET /api/health              Health check
GET /api/feed/health         Ingester status
GET /api/feed/               Paginated event feed
GET /api/feed/stats          Event counts
GET /api/feed/event/{id}     Single event
GET /api/feed/pr/{number}    PR events
GET /api/feed/issue/{number} Issue events
GET /api/feed/user/{user}    User events
GET /api/feed/voters         Voter leaderboard
GET /api/feed/voters/{user}  Individual voter
GET /api/feed/votes/pr/{n}   PR vote breakdown
```

## Running Locally

```bash
# Go API
export DATABASE_URL="postgres://..."
export GITHUB_TOKEN="ghp_..."
export GITHUB_REPO="skridlevsky/openchaos"
go run ./cmd/server

# Next.js frontend
cd web
npm install
NEXT_PUBLIC_API_URL=http://localhost:8080 npm run dev

# Backfill historical data
go run ./cmd/backfill
```

## Environment Variables

| Variable                      | Required | Default                 | Description                  |
| ----------------------------- | -------- | ----------------------- | ---------------------------- |
| `DATABASE_URL`                | Yes      | -                       | PostgreSQL connection string |
| `GITHUB_TOKEN`                | Yes      | -                       | GitHub personal access token |
| `GITHUB_REPO`                 | No       | `skridlevsky/openchaos` | Target repository            |
| `PORT`                        | No       | `8080`                  | Server port                  |
| `GITHUB_POLL_INTERVAL`        | No       | `60s`                   | Events API poll interval     |
| `GITHUB_REACTIONS_INTERVAL`   | No       | `5m`                    | Reactions poll interval      |
| `GITHUB_DISCUSSIONS_INTERVAL` | No       | `10m`                   | Discussions poll interval    |
| `NEXT_PUBLIC_API_URL`         | No       | `http://localhost:8080` | Go API URL (for frontend)    |

## License

MIT

## Analytics

This site uses [Vercel Analytics](https://vercel.com/analytics) — cookieless, privacy-friendly, no personal data collected.
