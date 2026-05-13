# Topline OS CLI Skill

Use the `topline` CLI for Topline OS CRM workflows that require multiple joins or compact reporting.

## Fast pipeline activity audit

For weekly qualified-pipeline questions, run one compound command instead of manually looping through opportunities, conversations, messages, and tasks:

```bash
topline --agent pipeline audit \
  --pipeline-id PIPE \
  --since this-week-et \
  --status open
```

The audit command resolves stages, paginates all open opportunities for the selected pipeline/status, scans recent conversations, then joins messages/tasks in parallel. If the recent scan is not deep enough for the window, it falls back to per-contact conversation lookups. Prefer this path before raw endpoint calls.

Before trusting a zero-activity result, confirm the returned JSON includes `activityJoinIncluded: true`. If that field is missing or false, the installed CLI is old or the audit was run with `--skip-activity`; treat the output as snapshot-only and run a fallback conversation/message join.

Use the returned `activityJoinStats` and `activeDeals` array first: it already includes opportunity names, stages, values, message counts, human/workflow counts, and per-deal activity counts.

Useful switches:

- `--skip-activity` for snapshot-only count/value/stage breakdown.
- `--conversation-limit 10` and `--message-limit 30` to tune lookup volume.
- `--include-tasks false` to skip overdue task hygiene.

## Setup and supporting commands

```bash
topline setup-check
topline --agent query schema
topline --agent query explain --tables opportunities,pipeline_stages,messages,contacts
topline --agent query sql --sql 'SELECT status, COUNT(*) AS n FROM opportunities GROUP BY status ORDER BY n DESC'
topline opportunities pipelines
topline opportunities search --pipeline-id PIPE --status open --limit 100
topline sync init --db topline.db
```

Use hosted SQL query commands for broad analytics when `TOPLINE_QUERY_TOKEN` is configured and warehouse freshness is acceptable. `TOPLINE_QUERY_TOKEN` must be a connection-bound token from `https://os-mcp.topline.com/connect`, not a raw PIT. Prefer `--agent` for token-efficient, PII-masked output. Never print PIT/API/query token values.
