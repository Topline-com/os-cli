---
name: topline-os-cli
description: Use the Topline OS CLI for token-efficient CRM operations, fast pipeline activity audits, deal briefs, and local SQLite-backed sales workflows.
version: 0.2.0
---

# Topline OS CLI

Use `topline` when the user asks for sales pipeline reporting, CRM hygiene, deal briefs, or repeated Topline OS reads where one compound CLI command is cheaper than many MCP/API/tool calls.

## Required env

- `TOPLINE_PIT`
- `TOPLINE_LOCATION_ID`
- `TOPLINE_BRAND_NAME` optional

Never print full PIT values. Mask secrets and unnecessary PII by default in summaries.

## First checks

```bash
topline setup-check
topline help
```

## Fast qualified-pipeline activity

For “what happened this week in our qualified pipeline?” use the compound audit first:

```bash
topline --agent pipeline audit \
  --pipeline-id PIPELINE_ID \
  --since this-week-et \
  --status open
```

`pipeline audit` performs the expensive join inside the CLI:

1. Pipeline/stage lookup.
2. Open opportunity search.
3. Recent conversation scan intersected with open pipeline contacts.
4. Recent message fetch for active conversations.
5. Overdue task fetch for active contacts.

If the recent conversation scan is not deep enough to cover the requested window, the CLI falls back to per-contact conversation lookups. Message/task reads are parallelized, so agents should not hand-roll sequential loops unless the command is missing a field they need.

The JSON must include `activityJoinIncluded: true` before you trust zero activity. It also includes `activityJoinStats` and `activeDeals` with opportunity name, stage, value, message count, human/workflow counts, and per-deal activity counts. Use those summaries directly before making any follow-up lookup.

If `activityJoinIncluded` is missing or false, the installed CLI is old or the audit was run with `--skip-activity`; treat the output as snapshot-only and run a fallback conversation/message join before saying there was no activity.

Use these flags when needed:

- `--skip-activity` — snapshot only: open count, value, stage breakdown.
- `--concurrency 8` — default parallelism; raise carefully, max effective cap is 16.
- `--recent-conversation-limit 100` — default global recent conversation scan depth; max API-safe value is 100.
- `--conversation-limit 10` — conversations fetched per opportunity contact.
- `--message-limit 30` — messages fetched per active conversation.
- `--include-tasks false` — skip overdue task hygiene lookup.

## Other preferred commands

```bash
topline opportunities pipelines
topline opportunities search --pipeline-id PIPELINE_ID --status open --limit 100
topline conversations search --contact-id CONTACT_ID --status all --limit 10
topline conversations messages --conversation-id CONVERSATION_ID --limit 10
topline sync init --db topline.db
```

## Reporting rule of thumb

Use raw MCP or `topline raw request` for one-off edge cases. Use the CLI for compound read/reporting workflows. Keep activity separate from movement: conversation activity can happen without opportunity stage/status changes.
