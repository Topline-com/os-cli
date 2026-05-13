# Topline OS CLI Skill

Use the `topline` CLI for Topline OS CRM workflows that require multiple joins or compact reporting.

## Fast pipeline activity audit

For weekly qualified-pipeline questions, run one compound command instead of manually looping through opportunities, conversations, messages, and tasks:

```bash
topline --agent pipeline audit \
  --pipeline-id PIPE \
  --since 2026-05-11 \
  --status open \
  --concurrency 8
```

The audit command resolves stages, pulls open opportunities, then joins per-contact conversations/messages/tasks in parallel. Prefer this path before raw endpoint calls.

Before trusting a zero-activity result, confirm the returned JSON includes `activityJoinIncluded: true`. If that field is missing or false, the installed CLI is old or the audit was run with `--skip-activity`; treat the output as snapshot-only and run a fallback conversation/message join.

Use the returned `activeDeals` array first: it already includes opportunity names, stages, values, message counts, human/workflow counts, and per-deal activity counts.

Useful switches:

- `--skip-activity` for snapshot-only count/value/stage breakdown.
- `--conversation-limit 10` and `--message-limit 30` to tune lookup volume.
- `--include-tasks false` to skip overdue task hygiene.

## Setup and supporting commands

```bash
topline setup-check
topline opportunities pipelines
topline opportunities search --pipeline-id PIPE --status open --limit 100
topline sync init --db topline.db
```

Prefer `--agent` for token-efficient, PII-masked output. Never print PIT/API token values.
