# AGENTS.md

This repo builds the Topline OS CLI.

Rules:

- Commit as Alex: `alex@topline.com`.
- Never commit PITs, API tokens, bearer tokens, or customer exports.
- Keep endpoint parity with `Topline-com/os-mcp` but bias new work toward compound sales workflows.
- Prefer test-first changes.
- Run `go test ./...` before committing.
