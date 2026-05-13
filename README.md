# Topline OS CLI

Agent-native command line interface for Topline OS.

`os-cli` is the Printing Press-style companion to [`Topline-com/os-mcp`](https://github.com/Topline-com/os-mcp): the MCP exposes safe CRM actions to agents; this CLI gives operators and agents fast, composable muscle memory for CRM work.

## Why this exists

Raw API and MCP tools are useful, but sales operations questions are usually compound:

- What happened this week in a qualified pipeline?
- Which deals have activity but no stage movement?
- Who needs follow-up today?
- Which tasks are stale or lying to the pipeline?
- What is the complete brief before I touch this deal?

A good CLI should answer those with one command, compact JSON, and optional local SQLite state instead of ten remote round trips.

## Install

```bash
go install github.com/Topline-com/os-cli/cmd/topline@latest
```

Local build:

```bash
git clone https://github.com/Topline-com/os-cli.git
cd os-cli
go build ./cmd/topline
```

## Auth

Set the same environment variables used by the MCP:

```bash
export TOPLINE_PIT="pit-..."
export TOPLINE_LOCATION_ID="your_location_id"
export TOPLINE_BRAND_NAME="Topline OS"
```

Optional for tests/proxies:

```bash
export TOPLINE_BASE_URL="https://services.leadconnectorhq.com"
```

## Quick start

```bash
topline setup-check
topline contacts search --query "Jane Doe" --limit 5
topline opportunities pipelines
topline opportunities search --pipeline-id CLUy1QapsrEeBiNrmQiL --status open --limit 100
topline conversations messages --conversation-id abc123 --limit 10
```

Agent-safe output:

```bash
topline --agent pipeline audit \
  --pipeline-id CLUy1QapsrEeBiNrmQiL \
  --since this-week-et \
  --status open
```

`pipeline audit` now performs the expensive CRM join inside the CLI: open
opportunities → recent conversations → recent messages → overdue tasks. The CLI
first scans the 100 most recent conversations and intersects them with open
pipeline contacts; if that scan is not deep enough to cover the requested window,
it falls back to per-contact conversation lookups. The JSON includes
`activityJoinIncluded: true`, `activityJoinStats`, and `activeDeals` summaries
with opportunity name, stage, value, message count, and per-deal activity counts,
so agents do not need a second lookup just to name the touched deals. If
`activityJoinIncluded` is missing or false, do not trust zero activity as a final
answer; run a fallback conversation/message join. Use `--skip-activity` when you
only need the open count/value/stage breakdown.

Local SQLite foundation:

```bash
topline sync init --db topline.db
```

Raw escape hatch:

```bash
topline raw request GET /contacts/ --query '{"limit":1}'
topline raw request POST /contacts/ --body '{"firstName":"Jane","email":"jane@example.com"}'
```

## Printing Press method

This repo is built around four rules:

1. Local SQLite beats repeated remote calls for compound questions.
2. Compound commands beat ten agent tool calls.
3. Agent-shaped JSON beats raw payload dumps.
4. Workflow commands should encode how sales operators actually work.

## Current scope

Parity command scaffolding exists for the public MCP action surface:

- contacts
- conversations/messages
- opportunities/pipelines
- calendars/appointments
- tasks
- notes
- custom fields
- custom values
- workflows
- tags
- users
- forms
- surveys
- location
- raw requests

Agent-native foundations included now:

- `pipeline audit` with recent conversation scan + parallel message/task joins
- `sync init`
- `--agent`
- `--mask-pii`
- compact JSON output
- SQLite schema for CRM mirror tables

## Design direction

Next commands should be high-level sales workflows, not endpoint wrappers:

```bash
topline deal brief --opportunity-id opp_123
topline followup queue --pipeline-id pipe_123 --since 2026-05-01
topline hygiene --pipeline-id pipe_123
topline activity rollup --pipeline-id pipe_123 --since 2026-05-11 --group-by owner
topline sync run --since 2026-05-01
```

## Development

```bash
go test ./...
go build ./cmd/topline
```

Commit identity for this repo should use Alex:

```bash
git config user.email alex@topline.com
git config user.name "Alex Skatell"
```

## License

MIT.
