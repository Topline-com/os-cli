---
name: topline-os-cli
description: Use the Topline OS CLI for token-efficient CRM operations, pipeline audits, deal briefs, and local SQLite-backed sales workflows.
version: 0.1.0
---

# Topline OS CLI

Use `topline` when the user asks for sales pipeline reporting, CRM hygiene, deal briefs, or repeated Topline OS reads where a CLI is cheaper than many MCP calls.

## Required env

- `TOPLINE_PIT`
- `TOPLINE_LOCATION_ID`
- `TOPLINE_BRAND_NAME` optional

Never print full PIT values. Mask secrets and PII by default in summaries.

## First checks

```bash
topline setup-check
topline help
```

## Preferred reporting commands

```bash
topline --agent pipeline audit --pipeline-id PIPELINE_ID --since YYYY-MM-DD
topline opportunities search --pipeline-id PIPELINE_ID --status open --limit 100
topline sync init --db topline.db
```

## Rule of thumb

Use raw MCP or `topline raw request` for one-off writes. Use the CLI for compound read/reporting workflows.
