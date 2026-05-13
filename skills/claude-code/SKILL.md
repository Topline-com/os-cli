# Topline OS CLI Skill

Use the `topline` CLI for Topline OS CRM workflows. For broad CRM analytics, **prefer the composite warehouse commands** (`topline --agent query audit|snapshot|freshness`) when `TOPLINE_QUERY_TOKEN` is configured. They wrap the standard pipeline audit shape into one CLI call. Use REST-backed CLI commands for live operational reads, exact object drilldowns, and approved writes.

## Standard pipeline audit contract (hard limit — 3 calls)

For any "what happened in pipeline X over window W" question, run **exactly** this sequence and stop:

1. `topline --agent query doctor` — readiness probe. JSON: `queryTokenPresent`, `schemaReachable`, `tableCount`, `missingTables`, `recommendation`. If `queryTokenPresent` is false, `schemaReachable` is false, or any expected table is missing, stop and report the readiness gap. Missing tables/views are `os-mcp` coverage bugs; surface them in the final answer.
2. `topline --agent query audit --pipeline PIPELINE_ID --since WINDOW --status open` — one composite call returning `freshness`, `snapshot`, `activity` (with `unique_touches`), `deals`, and `movement`. Default `--since this-week-et` for "this week"; the CLI resolves the window.
3. Answer.

**Hard ceiling: 3 tool calls after skills load** (doctor + audit + answer). Banned in the default flow:

- Raw `topline --agent query sql --sql ...` for standard pipeline audits — `query audit` already covers the shape.
- `query schema`, `query explain`, or per-table freshness SQL before `query audit`. The audit payload's `freshness` field is enough.
- `pipeline audit`, `opportunities search`, `conversations search`, per-conversation message fetches.
- `python3` / `execute_code` / any subprocess wrapper around `topline`. The CLI returns JSON; parse it directly.
- **Bash heredocs around `query sql`**: `SQL=$(cat <<'SQL' ... SQL)` or `topline --agent query sql --sql "$(cat <<SQL ... SQL)"`. Same shape as the Python wrapper anti-pattern, different shell. If you find yourself authoring multi-line SQL for a standard pipeline question, switch to `query audit`.
- Editing this skill (or the audits skill) via `skill_manage` mid-run. The contract is read-only during execution; propose edits in a separate turn.

Exceptions — each requires the user explicitly asking:

- "Drill into deal X" → `opportunities` / `conversations` / `messages` calls OK.
- "Why does SQL disagree with the audit?" → run `pipeline audit` and reconcile.
- "Is the warehouse stale?" → `topline --agent query freshness` or a targeted `MAX(_synced_at)` SQL.
- Non-standard analytics that `query audit` / `snapshot` / `freshness` cannot express → raw `query sql` is fine, but keep it inline (`--sql '...'`), not in a heredoc.

If `query doctor` or the audit payload's `freshness` rows flag stale data or coverage gaps, disclose it in the answer but do **not** auto-fallback to REST. Coverage gaps are bugs to fix in `os-mcp`, not workarounds for the agent.

## Composite warehouse commands (preferred for analytics)

```bash
topline --agent query doctor
topline --agent query audit --pipeline PIPELINE_ID --since this-week-et --status open
topline --agent query snapshot --pipeline PIPELINE_ID --status open
topline --agent query freshness
```

Answer from `query audit` JSON directly:

- `snapshot.rows` → open count/value and stage distribution.
- `activity.rows` → `unique_touches` by channel/direction.
- `deals.rows` → per-deal touch breakdowns.
- `movement.rows` → stage/status/record movement.
- `freshness.rows` → data freshness caveats.

Keep activity and movement separate.

`TOPLINE_QUERY_TOKEN` must be a connection-bound token from `https://os-mcp.topline.com/connect`; raw PITs are intentionally rejected for SQL. Never print query tokens.

## Raw SQL (non-standard analytics only)

```bash
topline --agent query explain --tables opportunities,pipeline_stages,messages,contacts,call_events,appointments
topline --agent query sql --sql 'SELECT status, COUNT(*) AS n FROM opportunities GROUP BY status ORDER BY n DESC'
```

Use raw SQL only for analytics the composite commands cannot express. Keep it inline; do not assemble multi-line SQL in a bash heredoc.

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

Do **not** run this after a successful `query audit` "to verify" — cross-verification is a drilldown subroutine, not the default.

## Setup and supporting commands

```bash
topline setup-check
topline opportunities pipelines
topline opportunities search --pipeline-id PIPE --status open --limit 100
topline sync init --db topline.db
```

## Output rules

Prefer `--agent` for token-efficient, PII-masked output. Never print PIT or query token values. Avoid markdown tables in chat replies; use bullets. Keep activity separate from movement: conversation activity can happen without opportunity stage/status changes.
