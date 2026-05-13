---
name: topline-os-cli
description: Use the Topline OS CLI for SQL-first CRM analytics, pipeline audits, token-efficient reads, deal briefs, and agent-safe sales operations. Default to the hosted warehouse `topline --agent query` path for analytics; use REST-backed commands for live drilldowns and approved writes.
version: 1.3.0
---

# Topline OS CLI

Use `topline` when the user asks for sales pipeline reporting, CRM hygiene, deal briefs, or any repeated Topline OS read where one compound SQL or CLI command is cheaper than many MCP/API/tool calls.

For broad CRM analytics, **prefer the hosted warehouse SQL path** (`topline --agent query ...`) when `TOPLINE_QUERY_TOKEN` is configured. Use REST-backed CLI commands for live operational reads, exact object drilldowns, and approved writes.

## Required env

- `TOPLINE_PIT`
- `TOPLINE_LOCATION_ID`
- `TOPLINE_BRAND_NAME` (optional)
- `TOPLINE_QUERY_TOKEN` (optional, enables hosted warehouse SQL via `topline --agent query ...`). Must be a connection-bound token from `https://os-mcp.topline.com/connect`; raw `TOPLINE_PIT` is intentionally rejected for SQL.

Never print full PIT or query token values. Mask secrets and unnecessary PII by default in summaries.

## First checks

```bash
topline setup-check
topline help
topline --agent query doctor      # deterministic SQL readiness probe (added in v1.3.0)
date '+%Y-%m-%d %H:%M:%S %Z (%z); %A; ISO week %V'
```

`query doctor` returns JSON with `queryTokenPresent`, `tokenSourceEnvVar`, `rawPitRejected`, `baseUrl`, `schemaReachable`, `tableCount`, `expectedTables`, `missingTables`, and a `recommendation`. It never prints the token. Use it as the first deterministic check before any analytics SQL.

## Standard pipeline audit contract (hard limit — 4 calls)

For any "what happened in pipeline X over window W" question, run **exactly** this sequence and stop:

1. `topline --agent query doctor` — ONE deterministic readiness probe. If `queryTokenPresent` is false or `schemaReachable` is false, stop and report the readiness gap. If `missingTables` is non-empty, treat each missing table as an `os-mcp` coverage bug and surface it in the final answer.
2. `topline --agent query sql --sql '<composite audit>'` — ONE call, returns: open count/value, stage rollup, activity by channel/direction, moved/created/closed deals in window, top active deals. If the window is custom/relative and not already known, prepend a single `date` call before this step.
3. Answer.

**Hard ceiling: 4 tool calls after skills load** (skills load + doctor + composite SQL + optional `date` + answer). No `pipeline audit`, no `opportunities search`, no `conversations search`, no per-conversation message fetches, no Python verification loops.

`query doctor` replaces the prior ad-hoc freshness/token probe — it is faster, deterministic, never prints the token, and surfaces both auth/readiness and warehouse coverage in one JSON payload. Only run a separate freshness SQL (`MAX(_synced_at)` per table) if the user explicitly asks about sync lag.

Exceptions — each requires the user explicitly asking:

- "Drill into deal X" → `opportunities` / `conversations` / `messages` calls OK.
- "Why does SQL disagree with the audit?" → run `pipeline audit` and reconcile.
- "Is the warehouse stale?" → freshness deep-dive (`MAX(_synced_at)` per table).

If `query doctor`'s `recommendation` flags stale data or coverage gaps, disclose it in the answer (e.g. "os-mcp missing `appointments` table; calls/appointments may be undercounted") but do **not** auto-fallback to REST. The user can ask for a live re-run if the gap matters. Coverage gaps are bugs to fix in `os-mcp`, not workarounds for the agent.

## Hosted warehouse SQL (preferred for analytics)

```bash
topline --agent query doctor
topline --agent query schema     # only when doctor flags schema not cached or stale
topline --agent query explain --tables opportunities,pipeline_stages,messages,contacts,call_events,appointments
topline --agent query sql --sql 'SELECT status, COUNT(*) AS n FROM opportunities GROUP BY status ORDER BY n DESC'
```

If `doctor` reports `queryTokenPresent: false` or `schemaReachable: false`, stop and ask the user to mint/configure a connection-bound query token. Do not silently fall back to REST and present it as the SQL-first result.

Qualified-pipeline weekly activity sample:

```bash
topline --agent query sql --sql "WITH qualified_opps AS (SELECT DISTINCT contact_id FROM opportunities WHERE pipeline_id = 'PIPELINE_ID' AND status = 'open' AND contact_id IS NOT NULL), week_messages AS (SELECT m.contact_id, m.type, m.direction, m.date_added FROM messages m JOIN qualified_opps q ON q.contact_id = m.contact_id WHERE m.date_added >= 'YYYY-MM-DDT00:00:00-04:00') SELECT COALESCE(type, 'unknown') AS activity_type, COALESCE(direction, 'unknown') AS direction, COUNT(*) AS touches, COUNT(DISTINCT contact_id) AS contacts_touched, MIN(date_added) AS first_touch, MAX(date_added) AS last_touch FROM week_messages GROUP BY activity_type, direction ORDER BY touches DESC LIMIT 20"
```

Rules:

- The command calls the hosted `Topline-com/os-mcp` `/query/api/*` endpoints and inherits MCP SQL safety: one `SELECT` / `WITH ... SELECT`, exposed tables only, 5,000-row cap.
- Use SQL first for counts, joins, stage/value snapshots, contact activity rollups, and funnel analytics. Use REST/CLI action commands for live mutations, exact object drilldowns, or when synced warehouse freshness is uncertain.
- Do not print query tokens. If missing, ask the user to mint/configure a connection-bound query token — do not silently fall back to REST and present it as the SQL-first result.

## Pipeline audit (diagnostic / one-off only — NOT the analytics default)

Use only when the user explicitly asks to reconcile SQL against live REST, drill into a specific deal beyond what SQL returned, or when `TOPLINE_QUERY_TOKEN` is missing/rejected:

```bash
topline --agent pipeline audit \
  --pipeline-id PIPELINE_ID \
  --since this-week-et \
  --status open
```

`pipeline audit` performs the expensive join inside the CLI:

1. Pipeline/stage lookup.
2. Paginated open opportunity search across every page for the selected pipeline/status.
3. Recent conversation scan intersected with open pipeline contacts.
4. Recent message fetch for active conversations.
5. Overdue task fetch for active contacts.

Confirm `activityJoinIncluded: true` in the JSON before trusting any zero-activity result. If that field is missing or false, the installed CLI is old or the audit was run with `--skip-activity`; treat the output as snapshot-only.

Do **not** run this after a successful SQL audit "to verify" — cross-verification is a drilldown subroutine, not the default.

Flags:

- `--skip-activity` — snapshot only: open count, value, stage breakdown.
- `--concurrency 8` — default parallelism; max effective cap is 16.
- `--recent-conversation-limit 100` — default global recent conversation scan depth; max API-safe value is 100.
- `--conversation-limit 10` — conversations fetched per opportunity contact.
- `--message-limit 30` — messages fetched per active conversation.
- `--include-tasks false` — skip overdue task hygiene lookup.

## Other CLI commands

```bash
topline opportunities pipelines
topline opportunities search --pipeline-id PIPELINE_ID --status open --limit 100
topline conversations search --contact-id CONTACT_ID --status all --limit 10
topline conversations messages --conversation-id CONVERSATION_ID --limit 10
topline contacts search --query "NAME_OR_EMAIL" --limit 10
topline contacts get --contact-id CONTACT_ID
topline tasks list --contact-id CONTACT_ID
topline notes list --contact-id CONTACT_ID
topline users list
topline sync init --db topline.db
topline raw request GET /opportunities/search --query '{"pipelineId":"PIPELINE_ID","status":"open","limit":100}'
```

## Output rules

- Prefer `--agent` for concise, token-efficient output.
- Add `--mask-pii` when sharing outside a private internal context.
- Use Python or SQL for arithmetic, grouping, and joins; never do CRM math mentally.
- Avoid markdown tables in chat replies (Discord/WhatsApp). Use bullets.
- Keep sales reporting separated into: activity, movement, hygiene, next action.

## Common pitfalls

1. **Starting CRM analytics with REST when SQL is available.** If `TOPLINE_QUERY_TOKEN` is configured and the task is a rollup/audit/count/join, use `topline --agent query sql` first. REST pagination and endpoint fan-out are fallback paths, not the default analytics path.
2. **Running both SQL AND `pipeline audit` for the same question.** The default audit ends after the SQL composite. Don't add `pipeline audit` "to verify" — cross-verification is a drilldown subroutine, not the default. Movement is already covered by the composite SQL via `last_stage_change_at` / `last_status_change_at` / `created_at`.
3. **Auto-falling back to REST when SQL is partial.** Hard rule: default audit ends after the SQL composite. Disclose any coverage gap in the answer; the user can ask for a live re-run if it matters. This keeps `os-mcp` view gaps visible as bugs instead of papered over.
4. **Mislabeling SQL/native disagreements as "sync lag".** Only call it lag when `_synced_at` proves lag. If SQL is fresh but a view is missing a UNION (e.g. appointments not yet in `contact_timeline`), say the SQL surface is incomplete for that metric (`os-mcp` coverage gap) and stop.
5. **Assuming opportunity updates equal activity.** Calls/SMS/email live in conversations/messages and may not move `updatedAt` or stage fields.
6. **Skipping stage lookup.** Stage IDs are opaque; map them through `opportunities pipelines` (or a SQL join on `pipeline_stages`) before presenting.
7. **Printing secrets or full PII.** PITs, query tokens, phone numbers, and credentials never belong in logs, memory, or summaries.
8. **Silent writes.** For mutating commands, get explicit approval and state the exact contact / opportunity / action first.
9. **Treating masked `--agent` JSON as machine-parseable for pagination.** `--agent` enables PII masking and may replace numeric pagination fields like `startAfter` with `[PHONE]`, which makes the output invalid JSON. For internal follow-up scripts, use unmasked CLI output into temp files, parse locally, and delete the files before finishing.

## Reporting rule of thumb

Use hosted SQL query commands for broad analytics when `TOPLINE_QUERY_TOKEN` is configured. Use `topline raw request` for one-off edge cases. Use the typed CLI commands for compound read/reporting workflows or live drilldowns. Keep activity separate from movement: conversation activity can happen without opportunity stage/status changes.
