# Topline OS CLI Skill

Use the `topline` CLI for Topline OS CRM workflows. For broad CRM analytics, **prefer the hosted warehouse SQL path** (`topline --agent query ...`) when `TOPLINE_QUERY_TOKEN` is configured. Use REST-backed CLI commands for live operational reads, exact object drilldowns, and approved writes.

## Standard pipeline audit contract (hard limit — 4 calls)

For any "what happened in pipeline X over window W" question, run **exactly** this sequence and stop:

1. `date` — only if the window is custom/relative and not already known.
2. `topline --agent query sql --sql '<freshness check>'` — ONE call covering `messages`, `opportunities`, `call_events`, `appointments` for the pipeline's contacts.
3. `topline --agent query sql --sql '<composite audit>'` — ONE call returning open count/value, stage rollup, activity by channel/direction, moved/created/closed deals in window, top active deals.
4. Answer.

**Hard ceiling: 4 tool calls after skills load.** No `pipeline audit`, no `opportunities search`, no `conversations search`, no per-conversation message fetches, no Python verification loops.

Exceptions — each requires the user explicitly asking:

- "Drill into deal X" → `opportunities` / `conversations` / `messages` calls OK.
- "Why does SQL disagree with the audit?" → run `pipeline audit` and reconcile.
- "Is the warehouse stale?" → freshness deep-dive.

If freshness shows lag > 30 min on any required table, disclose it in the answer but do **not** auto-fallback to REST. If SQL is fresh but a warehouse view is missing required coverage (e.g. appointments not yet UNION'd into `contact_timeline`), state that as an `os-mcp` coverage gap and stop. Do not paper coverage gaps with REST.

## Hosted warehouse SQL (preferred for analytics)

```bash
topline --agent query schema
topline --agent query explain --tables opportunities,pipeline_stages,messages,contacts,call_events,appointments
topline --agent query sql --sql 'SELECT status, COUNT(*) AS n FROM opportunities GROUP BY status ORDER BY n DESC'
```

`TOPLINE_QUERY_TOKEN` must be a connection-bound token from `https://os-mcp.topline.com/connect`; raw PITs are intentionally rejected for SQL. Never print query tokens.

## Pipeline audit (diagnostic / one-off only — NOT the analytics default)

Use only when the user explicitly asks to reconcile vs. live source, drill into a specific deal beyond what SQL returned, or when `TOPLINE_QUERY_TOKEN` is missing/rejected:

```bash
topline --agent pipeline audit \
  --pipeline-id PIPE \
  --since this-week-et \
  --status open
```

The audit command resolves stages, paginates all open opportunities for the selected pipeline/status, scans recent conversations, then joins messages/tasks in parallel. Before trusting a zero-activity result, confirm the returned JSON includes `activityJoinIncluded: true`. If that field is missing or false, the installed CLI is old or the audit was run with `--skip-activity`; treat the output as snapshot-only.

Useful switches:

- `--skip-activity` for snapshot-only count/value/stage breakdown.
- `--conversation-limit 10` and `--message-limit 30` to tune lookup volume.
- `--include-tasks false` to skip overdue task hygiene.

Do **not** run this after a successful SQL audit "to verify" — cross-verification is a drilldown subroutine, not the default.

## Setup and supporting commands

```bash
topline setup-check
topline opportunities pipelines
topline opportunities search --pipeline-id PIPE --status open --limit 100
topline sync init --db topline.db
```

## Output rules

Prefer `--agent` for token-efficient, PII-masked output. Never print PIT or query token values. Avoid markdown tables in chat replies; use bullets. Keep activity separate from movement: conversation activity can happen without opportunity stage/status changes.
