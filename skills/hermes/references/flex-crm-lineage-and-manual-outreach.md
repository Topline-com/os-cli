# Flex CRM lineage and manual outreach audit notes

Use this reference when Alex asks about Flex triage → qualified conversion, won deals, rep responsiveness, or manual outreach counts.

## Definitions

- **Manual outreach**: rep-authored outbound calls, emails, or SMS. Exclude workflow/app automation; in the warehouse, `raw_payload.source = 'app'` is the key automation signal observed in `messages`.
- **Response SLA**: for lead-response audits, measure the first outbound call within 4 hours of opportunity creation unless Alex specifies another SLA.
- **Owner attribution**: prefer `opportunities.assigned_to`; fall back to `contacts.assigned_to` when opportunity owner is missing.
- **Current month / QTD**: use the live date tool first, then build explicit date boundaries.

## Pipeline lineage caveat

The hosted warehouse `opportunities` table represents current opportunity state. A deal currently in `Sales - Flex - Qualified` with status `won` can be a good operational proxy for a triage-converted won deal, but it is not proof that the opportunity originally started in `Sales - Flex - Triage` unless a history/audit surface records the move.

Observed pipeline IDs:

- `Sales - Flex - Triage`: `bna6e9DoPgRchNsjeYS3`
- `Sales - Flex - Qualified`: `CLUy1QapsrEeBiNrmQiL`

When asked “how many Triage leads won,” do not only query current Triage; also query current Qualified wins in the window, then state the limitation:

- Current Triage + won: direct same-pipeline wins.
- Current Qualified + won + created/closed in window: operational proxy for moved-forward wins.
- True origin lineage: requires opportunity-history/audit table or explicit activity event; if unavailable, say so plainly.

## Useful SQL patterns

Manual outreach by rep/month:

```sql
SELECT
  assigned_user_name,
  COUNT(*) AS manual_touches,
  COUNT(DISTINCT contact_id) AS contacts_touched,
  SUM(CASE WHEN channel = 'call' THEN 1 ELSE 0 END) AS calls,
  SUM(CASE WHEN channel = 'email' THEN 1 ELSE 0 END) AS emails,
  SUM(CASE WHEN channel = 'sms' THEN 1 ELSE 0 END) AS sms
FROM messages
WHERE direction = 'outbound'
  AND created_at >= 'YYYY-MM-01'
  AND created_at < 'YYYY-MM-NEXT-01'
  -- adapt to the warehouse JSON/text dialect; exclude automation where raw_payload.source = 'app'
  AND COALESCE(JSON_EXTRACT(raw_payload, '$.source'), '') <> 'app'
GROUP BY assigned_user_name;
```

Flex Qualified won proxy:

```sql
SELECT
  contact_name,
  company_name,
  value,
  source,
  created_at,
  closed_at
FROM opportunities
WHERE pipeline_id = 'CLUy1QapsrEeBiNrmQiL'
  AND status = 'won'
  AND created_at >= 'YYYY-01-01'
ORDER BY closed_at;
```

Before relying on history, check whether the warehouse exposes a movement surface:

```sql
SELECT name
FROM sqlite_master
WHERE LOWER(name) LIKE '%history%'
   OR LOWER(name) LIKE '%audit%'
   OR LOWER(name) LIKE '%movement%';
```

If no history surface exists and activity rows do not include stage/pipeline move payloads, qualify the answer as current-state/proxy analysis, not proven origin lineage.
