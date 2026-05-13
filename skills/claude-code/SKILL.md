# Topline OS CLI Skill

Use the `topline` CLI for Topline OS CRM workflows. For broad CRM analytics, **prefer the hosted warehouse SQL path** (`topline --agent query ...`) when `TOPLINE_QUERY_TOKEN` is configured. Use REST-backed CLI commands for live operational reads, exact object drilldowns, and approved writes.

## Standard pipeline audit contract (hard limit — 4 calls)

For any "what happened in pipeline X over window W" question, run **exactly** this sequence and stop:

1. Readiness probe. Prefer `topline --agent query doctor` when the installed CLI supports it; JSON: `queryTokenPresent`, `schemaReachable`, `tableCount`, `missingTables`, `recommendation`. If the command returns `unknown query command "doctor"`, the binary is stale — rebuild via `scripts/install-local.sh`, or use `topline --agent query help` + `topline --agent query explain --tables opportunities,pipeline_stages,messages,contacts,call_events,appointments` as the fallback probe and proceed with SQL. If `queryTokenPresent` is false, `schemaReachable` is false, or `explain` fails for an expected table, stop and report the readiness gap. If `missingTables` is non-empty, treat each as an `os-mcp` coverage bug and surface it in the final answer.
2. `topline --agent query sql --sql '<composite audit>'` — ONE call returning open count/value, stage rollup, activity by channel/direction, moved/created/closed deals in window, top active deals. If the window is custom/relative and not already known, prepend a single `date` call before this step.
3. Answer.

**Hard ceiling: 4 tool calls after skills load** (date if needed + readiness probe + composite SQL + answer). Banned in the default flow:

- `pipeline audit`, `opportunities search`, `conversations search`, per-conversation message fetches.
- `python3` / `execute_code` / any subprocess wrapper around `topline`. The CLI returns JSON; parse it directly. Reshape data in SQL with a final `SELECT`, not in Python.
- Editing this skill (or the audits skill) via `skill_manage` mid-run. The contract is read-only during execution; propose edits in a separate turn.

`query doctor` replaces the prior ad-hoc freshness/token probe. Only run a separate freshness SQL (`MAX(_synced_at)` per table) if the user explicitly asks about sync lag.

Exceptions — each requires the user explicitly asking:

- "Drill into deal X" → `opportunities` / `conversations` / `messages` calls OK.
- "Why does SQL disagree with the audit?" → run `pipeline audit` and reconcile.
- "Is the warehouse stale?" → freshness deep-dive (`MAX(_synced_at)` per table).

If `query doctor`'s `recommendation` flags stale data or coverage gaps, disclose it in the answer but do **not** auto-fallback to REST. Coverage gaps are bugs to fix in `os-mcp`, not workarounds for the agent.

## Hosted warehouse SQL (preferred for analytics)

```bash
topline --agent query doctor
topline --agent query schema     # only when doctor flags schema not cached or stale
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
