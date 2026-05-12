# Topline OS CLI Skill

Use the `topline` CLI for Topline OS CRM workflows that require multiple joins or compact reporting.

Examples:

```bash
topline setup-check
topline --agent pipeline audit --pipeline-id PIPE --since 2026-05-11
topline opportunities search --pipeline-id PIPE --status open --limit 100
topline sync init --db topline.db
```

Prefer `--agent` for token-efficient, PII-masked output.
