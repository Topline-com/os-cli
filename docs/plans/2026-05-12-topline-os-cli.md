# Topline OS CLI Implementation Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Build a first-principles, Printing Press-style CLI companion to Topline OS MCP with endpoint parity plus agent-native CRM workflow commands.

**Architecture:** A Go binary (`topline`) uses the same PIT/location auth model as `os-mcp`, exposes MCP parity commands through a command-spec registry, and adds compound workflow commands backed by a local SQLite mirror. The CLI favors compact JSON, PII masking, and reusable operator workflows over raw API-shaped payloads.

**Tech Stack:** Go, standard-library CLI parser, `net/http`, `database/sql`, `modernc.org/sqlite`, GitHub Actions.

---

### Task 1: Bootstrap the Go module

**Objective:** Create the repository skeleton and test-first package boundaries.

**Files:**
- Create: `go.mod`
- Create: `cmd/topline/main.go`
- Create: `internal/topline/config_test.go`
- Create: `internal/topline/client_test.go`

**Verification:** Run `go test ./...` and confirm initial tests fail because implementation is missing.

### Task 2: Implement auth and HTTP client

**Objective:** Load `TOPLINE_PIT`, `TOPLINE_LOCATION_ID`, and send LeadConnector-compatible API requests.

**Files:**
- Create: `internal/topline/config.go`
- Create: `internal/topline/client.go`

**Verification:** Run `go test ./internal/topline`.

### Task 3: Implement output safety

**Objective:** Add JSON output helpers and PII/secret masking for agent mode.

**Files:**
- Create: `internal/output/pii_test.go`
- Create: `internal/output/pii.go`
- Create: `internal/output/json.go`

**Verification:** Run `go test ./internal/output`.

### Task 4: Build MCP parity command registry

**Objective:** Map every current `os-mcp` action tool to a CLI command and raw request shape.

**Files:**
- Create: `internal/commands/specs_test.go`
- Create: `internal/commands/specs.go`
- Create: `internal/commands/execute.go`

**Verification:** Run `go test ./internal/commands` and `go build ./cmd/topline`.

### Task 5: Add Printing Press workflow foundations

**Objective:** Add the first compound sales workflow and SQLite mirror foundation.

**Files:**
- Create: `internal/reports/pipeline_audit_test.go`
- Create: `internal/reports/pipeline_audit.go`
- Create: `internal/sync/schema_test.go`
- Create: `internal/sync/schema.go`
- Create: `internal/commands/pipeline.go`

**Verification:** Run `go test ./...`, `topline sync init --db /tmp/topline-os-cli-test.db`, and `topline help`.

### Task 6: Document and publish

**Objective:** Create public repo docs, CI, skill files, and push to GitHub.

**Files:**
- Create: `README.md`
- Create: `docs/parity.md`
- Create: `docs/examples.md`
- Create: `skills/hermes/SKILL.md`
- Create: `.github/workflows/ci.yml`
- Create: `LICENSE`

**Verification:** Run `go test ./...`, commit with `alex@topline.com`, create `Topline-com/os-cli`, push, and verify `gh repo view Topline-com/os-cli`.
