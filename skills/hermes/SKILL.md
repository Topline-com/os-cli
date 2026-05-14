---
name: topline-os-cli
description: Use the Topline OS CLI for SQL-first CRM analytics, pipeline audits, token-efficient reads, deal briefs, and agent-safe sales operations. Default to the composite `topline --agent query audit|snapshot|freshness` commands for standard analytics; use REST-backed commands for live drilldowns and approved writes.
version: 1.6.1
---

# Topline OS CLI

Use `topline` when the user asks for sales pipeline reporting, CRM hygiene, deal briefs, or any repeated Topline OS read where one composite CLI command is cheaper than many MCP/API/tool calls.

For broad CRM analytics, **prefer the composite warehouse commands** (`topline --agent query audit|snapshot|freshness`) when `TOPLINE_QUERY_TOKEN` is configured. They wrap the standard pipeline audit shape (snapshot, activity, deals, movement, freshness) into one CLI call, so agents do not need to hand-write SQL for the common case. Use REST-backed CLI commands for live operational reads, exact object drilldowns, and approved writes.

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
topline --agent query doctor      # deterministic SQL readiness probe (v1.3.0+)
date '+%Y-%m-%d %H:%M:%S %Z (%z); %A; ISO week %V'
```

`query doctor` returns JSON with `queryTokenPresent`, `tokenSourceEnvVar`, `rawPitRejected`, `baseUrl`, `schemaReachable`, `tableCount`, `expectedTables`, `missingTables`, and a `recommendation`. It never prints the token. Use it as the first deterministic check before any analytics SQL.

## Standard pipeline audit contract (hard limit — 3 calls)

For any "what happened in pipeline X over window W" question, run **exactly** this sequence and stop:

1. `topline --agent query doctor` — readiness probe. If `queryTokenPresent` is false, `schemaReachable` is false, or any expected table is missing, stop and report the readiness gap. Missing tables/views are `os-mcp` coverage bugs; surface them in the final answer.
2. `topline --agent query audit --pipeline PIPELINE_ID --since WINDOW --status open` — one composite call returning `freshness`, `snapshot`, `activity` (with `unique_touches`), `deals` (per-deal touch breakdowns), and `movement`. Default `--since this-week-et` for "this week"; the CLI resolves the window so a separate `date` call is not needed for that phrase.
3. Answer.

**Hard ceiling: 3 tool calls after skills load** (doctor + audit + answer). The following are explicitly banned in the default flow:

- Raw `topline --agent query sql --sql ...` for standard pipeline audits — the composite `query audit` already covers the shape.
- `query schema`, `query explain`, or per-table freshness SQL before `query audit`. Use `query doctor` for readiness and rely on the audit payload's `freshness` field.
- `pipeline audit`, `opportunities search`, `conversations search`, per-conversation message fetches.
- `python3` / `execute_code` / any subprocess wrapper around `topline` calls. The CLI already returns JSON; parse it in the answer, not in a Python loop.
- **Bash heredocs around `query sql`**: `SQL=$(cat <<'SQL' ... SQL)` or `topline --agent query sql --sql "$(cat <<SQL ... SQL)"`. Same shape as the Python wrapper anti-pattern, different shell. If you find yourself authoring multi-line SQL in a heredoc for a standard pipeline question, switch to `query audit`.
- **Post-hoc computation on the `query audit` JSON.** No `python3 - <<'PY' vals=[...]` averages, no `jq` sums, no `awk` totals, no inline bash math over the audit payload. The audit response already contains `activity` (totals + by-stage), `deals` (rollups), `movement` (counts + classification), `snapshot` (avg days in stage, value totals), and `freshness`. If the answer needs a number that isn't in the payload, **express it as SQL and call `topline --agent query sql`** — do not compute it in Python/jq/bash on the JSON the audit just returned. Computing in a wrapper is the same "I rejected the contract" signal as wrapping the CLI in the first place; it just moves the violation past the CLI boundary.
- `skill_manage` edits to this skill or `topline-os-crm-audits` during the audit. The contract is **read-only during execution**. If the skill is wrong, finish the current run honestly (or stop and disclose the gap), then propose the edit in a separate turn.

Exceptions — each requires the user explicitly asking:

- "Drill into deal X" → `opportunities` / `conversations` / `messages` calls OK.
- "Why does SQL disagree with the audit?" → run `pipeline audit` and reconcile.
- "Is the warehouse stale?" → run `topline --agent query freshness` or a targeted `MAX(_synced_at)` SQL.
- Non-standard analytics that `query audit` / `query snapshot` / `query freshness` cannot express → raw `query sql` is fine, but author the SQL inline (`--sql '...'`), not in a heredoc.

If `query doctor` or the audit payload's `freshness` rows flag stale data or coverage gaps, disclose it in the answer but do **not** auto-fallback to REST. Coverage gaps are bugs to fix in `os-mcp`, not workarounds for the agent.

## Composite warehouse commands (preferred for analytics)

```bash
topline --agent query doctor
topline --agent query audit --pipeline PIPELINE_ID --since this-week-et --status open
topline --agent query snapshot --pipeline PIPELINE_ID --status open
topline --agent query freshness
```

What each returns:

- `query doctor` — auth/readiness JSON: `queryTokenPresent`, `tokenSourceEnvVar`, `rawPitRejected`, `baseUrl`, `schemaReachable`, `tableCount`, `expectedTables`, `missingTables`, `recommendation`.
- `query audit` — `freshness`, `snapshot`, `activity` (with `unique_touches`), `deals`, `movement`. The standard pipeline audit payload.
- `query snapshot` — open count/value and stage distribution (subset of `audit.snapshot`).
- `query freshness` — `_synced_at` lag per warehouse table/view.

Answer from `query audit` JSON directly:

- `snapshot.rows` → open count/value and stage distribution.
- `activity.rows` → `unique_touches` by channel/direction. Use `unique_touches`, not raw row counts.
- `deals.rows` → per-deal touch breakdowns.
- `movement.rows` → stage/status/record movement.
- `freshness.rows` → data freshness caveats.

Keep activity and movement separate: messages/call_events/appointments prove touches; movement rows prove stage/status/record movement.

## Raw SQL (non-standard analytics only)

```bash
topline --agent query explain --tables opportunities,pipeline_stages,messages,contacts,call_events,appointments
topline --agent query sql --sql 'SELECT status, COUNT(*) AS n FROM opportunities GROUP BY status ORDER BY n DESC'
```

Raw SQL is for analytics the composite commands cannot express (e.g. unusual rollups, cross-pipeline funnels, custom user-supplied joins). Keep it inline; do not assemble multi-line SQL in a bash heredoc.

Rules:

- The command calls the hosted `Topline-com/os-mcp` `/query/api/*` endpoints and inherits MCP SQL safety: one `SELECT` / `WITH ... SELECT`, exposed tables only, 5,000-row cap.
- Use SQL first for counts, joins, stage/value snapshots, contact activity rollups, and funnel analytics that the composite commands do not cover. Use REST/CLI action commands for live mutations, exact object drilldowns, or when synced warehouse freshness is uncertain.
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

Do **not** run this after a successful `query audit` "to verify" — cross-verification is a drilldown subroutine, not the default.

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
- Use composite query commands or inline SQL for arithmetic, grouping, and joins; never do CRM math mentally.
- Avoid markdown tables in chat replies (Discord/WhatsApp). Use bullets.
- Keep sales reporting separated into: activity, movement, hygiene, next action.

## Common pitfalls

1. **Starting CRM analytics with REST or raw SQL when a composite command is available.** If `TOPLINE_QUERY_TOKEN` is configured and the task is a standard pipeline rollup/audit, use `topline --agent query audit` (or `query snapshot` / `query freshness`) first. REST pagination, endpoint fan-out, and hand-written SQL are fallback/advanced paths.
2. **Running both `query audit` AND `pipeline audit` for the same question.** The default audit ends after `query audit`. Movement is already included in `query audit.movement`; don't add `pipeline audit` "to verify."
3. **Auto-falling back to REST when SQL is partial.** Hard rule: default audit ends after `query audit`. Disclose any coverage gap in the answer; the user can ask for a live re-run if it matters. This keeps `os-mcp` view gaps visible as bugs instead of papered over.
4. **Mislabeling SQL/native disagreements as "sync lag".** Only call it lag when `_synced_at` proves lag. If SQL is fresh but a view is missing a UNION (e.g. appointments not yet in `contact_timeline`), say the SQL surface is incomplete for that metric (`os-mcp` coverage gap) and stop.
5. **Assuming opportunity updates equal activity.** Calls/SMS/email live in conversations/messages/`call_events` and may not move `updatedAt` or stage fields.
6. **Skipping stage lookup.** Stage IDs are opaque; map them through `opportunities pipelines` (or a SQL join on `pipeline_stages`) before presenting.
7. **Printing secrets or full PII.** PITs, query tokens, phone numbers, and credentials never belong in logs, memory, or summaries.
8. **Silent writes.** For mutating commands, get explicit approval and state the exact contact / opportunity / action first.
9. **Treating masked `--agent` JSON as machine-parseable for pagination.** `--agent` enables PII masking and may replace numeric pagination fields like `startAfter` with `[PHONE]`, which makes the output invalid JSON. For internal follow-up scripts, use unmasked CLI output into temp files, parse locally, and delete the files before finishing.
10. **Wrapping `topline` in `python3` / `execute_code` / `subprocess.run` to "verify" or "reshape" CLI output.** The CLI already returns JSON; parse it directly in the answer. Python wrapper loops are the new shape of the old REST-fan-out anti-pattern.
11. **Wrapping `query sql` in a bash heredoc.** `SQL=$(cat <<'SQL' ... SQL)` or `topline --agent query sql --sql "$(cat <<SQL ... SQL)"` is the same anti-pattern as Python wrapper loops in a different shell. If you are authoring multi-line SQL for a standard pipeline question, switch to `query audit`. If the question genuinely needs raw SQL, keep it inline on `--sql '...'`.
12. **Editing this skill (or `topline-os-crm-audits`) mid-audit via `skill_manage`.** The contract is read-only during execution. If the skill is wrong, finish the current run honestly (or stop and disclose the gap), then propose the edit in a follow-up turn.
13. **Prompt-rule tightening instead of primitive design.** If repeated runs keep finding new over-calling shapes (REST fan-out → python wrappers → over-decomposed SQL → bash heredocs), stop adding rules and move the workflow into a composite command/view. The standard pipeline audit is now `query doctor` → `query audit` → answer.
14. **Doing math on the audit JSON after the fact.** Real failure mode: agent runs `query doctor` + `query audit` cleanly, then opens a `python3 - <<'PY' vals=[...] PY` heredoc (or `jq` / `awk` / bash arithmetic) to compute averages/totals over the audit's `activity.by_stage`, `deals`, or `snapshot` rollups before answering. The audit response already carries those rollups — `activity.total_messages`, `activity.by_stage[*]`, `deals.open_count`, `deals.open_value_total`, `snapshot.avg_days_in_stage`, `movement.advances`/`movement.regresses`/`movement.stalls`. If a question genuinely needs a number not in the payload (e.g. p95 instead of avg), express it as `topline --agent query sql --sql ...` and disclose that it is non-standard analytics. Computing in Python/jq/bash on the audit JSON is the same anti-pattern as wrapping the CLI in Python — it just moves the violation one step past the CLI boundary.
15. **Treating current pipeline as historical origin.** The `opportunities` warehouse table exposes current pipeline/stage state; it does not, by itself, prove that a won Qualified opportunity started in Triage. For Flex conversion questions, answer in two layers: (a) direct/current-state query (current pipeline = `Sales - Flex - Qualified`, status = won, created/closed in window); (b) lineage confidence caveat unless a history/audit table or activity event explicitly records the pipeline move. Do not report "originated in Triage" as proven just because the deal is now in Qualified. See `references/flex-crm-lineage-and-manual-outreach.md`.
16. **Counting automated workflow touches as rep effort.** For manual outreach/activity audits, exclude workflow/app automation. In the hosted warehouse, use message/activity metadata such as `raw_payload.source = 'app'` as the automation exclusion signal when available, then break out calls/email/SMS separately and report contact counts.

## Reporting rule of thumb

Use composite query commands for broad analytics when `TOPLINE_QUERY_TOKEN` is configured. Use raw `query sql` for non-standard analytics. Use the typed CLI commands for compound read/reporting workflows or live drilldowns. Keep activity separate from movement: conversation activity can happen without opportunity stage/status changes.
