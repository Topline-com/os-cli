# Examples

## Qualified pipeline activity this week

```bash
topline --agent pipeline audit \
  --pipeline-id CLUy1QapsrEeBiNrmQiL \
  --since this-week-et \
  --status open
```

This single command resolves the pipeline/stage map, paginates through all open
opportunities for the selected pipeline/status, scans recent conversations, then
joins recent messages and overdue tasks in parallel. If the recent conversation
scan is not deep enough to cover the window, the CLI falls back to per-contact
conversation lookups. The response includes
`activityJoinIncluded: true`, `activityJoinStats`, and `activeDeals` with deal
names, stages, values, and per-deal activity counts so an agent can answer the
sales question directly. If `activityJoinIncluded` is missing or false, use a
fallback conversation/message join before reporting zero activity.
Use `--skip-activity` for a fast snapshot-only count/value/stage breakdown.

## Warehouse SQL analytics

```bash
export TOPLINE_QUERY_TOKEN="signed_connection_token_from_/connect"

topline --agent query schema

topline --agent query explain --tables opportunities,pipeline_stages,messages

topline --agent query sql --sql '
  SELECT p.name AS pipeline, COUNT(*) AS open_deals, SUM(o.monetary_value) AS value
  FROM opportunities o
  JOIN pipelines p ON p.id = o.pipeline_id
  WHERE o.status = "open"
  GROUP BY p.name
  ORDER BY value DESC
'
```

Use `query` for broad analytics and audits when SQL over the synced warehouse is
better than paginating live REST endpoints. The hosted MCP query API enforces
read-only SQLite: one `SELECT` / `WITH ... SELECT`, exposed tables only, 5,000-row
cap.

## Search open opportunities

```bash
topline opportunities search \
  --pipeline-id CLUy1QapsrEeBiNrmQiL \
  --status open \
  --limit 100
```

## Pull recent messages

```bash
topline conversations messages \
  --conversation-id qwuPtEkodCM8k232NsWg \
  --limit 10 \
  --mask-pii
```

## Create a task

```bash
topline tasks create \
  --contact-id CONTACT_ID \
  --title "Follow up" \
  --due-date 2026-05-13T16:00:00-04:00
```

## Initialize local mirror DB

```bash
topline sync init --db topline.db
```
