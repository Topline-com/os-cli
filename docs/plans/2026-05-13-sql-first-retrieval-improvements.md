# SQL-First Topline OS Retrieval Improvements Plan

> **For Hermes:** Use subagent-driven-development skill to implement this plan task-by-task.

**Goal:** Make Topline OS CRM questions feel like deterministic, SQL-first retrieval instead of clunky REST pagination, one-off SQL construction, and agent memory of table/field names.

**Architecture:** Keep hosted `Topline-com/os-mcp` warehouse SQL as the analytics source. Improve the local Go CLI so agents call stable high-level report/query commands that compile to safe SQL templates, include freshness/evidence metadata, and fall back to REST only when SQL is unavailable or freshness is unacceptable. Keep writes/mutations outside this path.

**Tech Stack:** Go CLI (`/Users/oc/workspace/os-cli`), hosted os-mcp HTTP query API, SQLite SQL dialect, Go tests with `httptest`, GitHub Actions, Hermes skills.

---

## Current Findings

- GitHub PR #3, `Add hosted warehouse query commands`, is merged and green.
- Local checkout is still on `feat/query-api-cli`; local `main` is behind `origin/main`.
- `go test ./...` passes locally on the current checkout.
- The local wrapper at `/Users/oc/.local/bin/topline` did not load `TOPLINE_QUERY_TOKEN` from Hermes `.env` files. This would make new Discord-thread tests fail SQL even if the token exists in profile env. The wrapper has been locally patched to allow `TOPLINE_QUERY_TOKEN`, `TOPLINE_QUERY_BASE_URL`, `TOPLINE_MCP_ACCESS_TOKEN`, and `TOPLINE_MCP_TOKEN`.
- No full query token should be committed, printed, or stored in skills/docs. Persist it to profile `.env` only with explicit approval.
- There are unrelated untracked plan docs in `docs/plans/`; do not commit them unless Alex explicitly wants them included.

## Product Diagnosis

The current CLI is directionally right but still clunky because:

1. Agents still have to decide when to use SQL, discover table names, and hand-write SQL.
2. The SQL surface is exposed as generic `topline query sql`, not as sales-native commands.
3. Output is raw query JSON, not an operator answer with facts, evidence, freshness, and caveats.
4. Pipeline names/stages/users still require manual lookup or remembered IDs.
5. There is no `doctor`/readiness command that proves: token present, hosted SQL reachable, schema available, and expected tables synced.
6. Local/GitHub state can drift after PRs merge, so agents may use an old branch or binary.

## Non-Negotiables

- SQL first for analytics: counts, rollups, joins, movement, funnel, contact activity.
- REST first only for live writes, exact operational object reads, and SQL freshness gaps.
- No raw PIT for SQL. SQL requires connection-bound query token.
- No secrets in docs, commits, skills, issue bodies, test fixtures, or terminal output.
- No LLM embedded inside the CLI for v1. The CLI should be deterministic. Let the agent choose high-level commands; the CLI compiles those commands to vetted SQL templates.
- Report activity and opportunity movement separately.

---

## Phase 0: Local Baseline and Release Hygiene

### Task 0.1: Sync local repo to merged GitHub state

**Objective:** Start all future work from `origin/main`, not the already-merged feature branch.

**Files:** none.

**Steps:**

```bash
cd /Users/oc/workspace/os-cli
git fetch origin --prune
git switch main
git pull --ff-only origin main
git status --short
```

**Expected:** `main` points at the merge commit for PR #3. Untracked docs may remain; do not stage them accidentally.

### Task 0.2: Install current binary locally

**Objective:** Ensure `/Users/oc/.local/bin/topline-bin` matches merged `origin/main`.

**Files:** none.

**Steps:**

```bash
cd /Users/oc/workspace/os-cli
go test ./...
go build -o /Users/oc/.local/bin/topline-bin ./cmd/topline
/opt/homebrew/bin/topline --agent query help
```

**Expected:** tests pass; help prints query commands.

### Task 0.3: Confirm query-token wiring without printing secrets

**Objective:** Verify the wrapper can load query-token keys from profile `.env` files.

**Files:**
- Already locally patched: `/Users/oc/.local/bin/topline`

**Steps:**

```bash
tmp=$(mktemp -d)
cat > "$tmp/.env" <<'EOF'
TOPLINE_PIT=pit-test
TOPLINE_LOCATION_ID=loc-test
TOPLINE_QUERY_TOKEN=signed-query-token-test
EOF
HERMES_HOME="$tmp" /opt/homebrew/bin/topline --agent query schema --url http://127.0.0.1:9 >/tmp/out 2>/tmp/err || true
! grep -q 'TOPLINE_QUERY_TOKEN is required' /tmp/err
rm -rf "$tmp" /tmp/out /tmp/err
```

**Expected:** command fails by connection/auth, not by missing token.

### Task 0.4: Persist the live query token only after approval

**Objective:** Store `TOPLINE_QUERY_TOKEN` in the relevant Hermes profile env files so new Discord threads work.

**Files:**
- `/Users/oc/.hermes/.env`
- `/Users/oc/.hermes/profiles/sales_agent/.env`
- optionally `/Users/oc/.hermes/profiles/marketing_agent/.env` and `/Users/oc/.hermes/profiles/cfo_agent/.env`

**Steps:**

```bash
# Use an editor or a script that never prints the value.
# Add exactly:
# TOPLINE_QUERY_TOKEN=<connection-bound token>
```

**Expected:**

```bash
HERMES_HOME=/Users/oc/.hermes/profiles/sales_agent \
  /opt/homebrew/bin/topline --agent query schema >/tmp/schema.json
python3 - <<'PY'
import json
json.load(open('/tmp/schema.json'))
print('schema_ok')
PY
rm -f /tmp/schema.json
```

---

## Phase 1: Add `topline query doctor`

### Task 1.1: Add query readiness tests

**Objective:** Prove agents can run one command to diagnose SQL readiness.

**Files:**
- Modify: `internal/commands/query_test.go`
- Modify: `internal/commands/query.go`

**Test cases:**

- Missing token returns `queryTokenPresent: false` and human guidance.
- Raw `pit-` token returns `rawPitRejected: true`.
- Valid fake token against `httptest.Server` calls `/query/api/get-overview`.
- Response includes table count and expected table presence when server returns schema.

**Command target:**

```bash
topline --agent query doctor
```

**Output shape:**

```json
{
  "queryTokenPresent": true,
  "baseUrl": "https://os-mcp.topline.com",
  "schemaReachable": true,
  "tableCount": 12,
  "expectedTables": {
    "contacts": true,
    "opportunities": true,
    "messages": true,
    "pipeline_stages": true
  },
  "recommendation": "SQL analytics ready"
}
```

### Task 1.2: Implement `doctor`

**Objective:** Make readiness self-evident before agents choose SQL or REST.

**Implementation notes:**

- Do not print token values.
- Reuse `topline.LoadQueryConfig()` but provide a mode that reports missing token instead of returning only an error.
- Call schema endpoint only if token looks valid.
- Keep output JSON and PII-safe.

**Verification:**

```bash
go test ./internal/commands -run Query
go test ./...
```

---

## Phase 2: Add SQL Template Registry

### Task 2.1: Create a deterministic query-template package

**Objective:** Stop making agents hand-write common SQL from memory.

**Files:**
- Create: `internal/queries/templates.go`
- Create: `internal/queries/templates_test.go`

**Template metadata:**

```go
type Template struct {
    Name        string
    Description string
    Tables      []string
    Params      []Param
    SQL         string
}
```

**Initial templates:**

- `pipeline.activity_by_channel`
- `pipeline.stage_value_snapshot`
- `pipeline.movement`
- `pipeline.active_deals`
- `contact.activity_rollup`
- `owner.pipeline_snapshot`

### Task 2.2: Add safe parameter binding/rendering

**Objective:** Allow parameterized templates without SQL injection or quoting mistakes.

**Rules:**

- Parameters are values only, never identifiers.
- Pipeline/stage/user identifiers must be resolved before template rendering.
- Strings are single-quoted and escaped.
- Dates are ISO strings computed by the CLI from human windows like `this-week-et`.

**Verification:** template tests assert exact SQL output and reject unknown params.

### Task 2.3: Add template listing and execution commands

**Commands:**

```bash
topline --agent query templates
topline --agent query template pipeline.activity_by_channel \
  --param pipeline_id=CLUy1QapsrEeBiNrmQiL \
  --param since=2026-05-11T00:00:00-04:00
```

**Output:** include `template`, `params`, `sql`, and query result.

---

## Phase 3: Add Sales-Native Report Commands

### Task 3.1: Add pipeline resolver

**Objective:** Let agents use names like `Sales - Flex - Qualified` without remembering IDs.

**Files:**
- Create: `internal/reports/resolver.go`
- Create: `internal/reports/resolver_test.go`

**Behavior:**

- SQL path: query `pipelines` + `pipeline_stages` and fuzzy-match names.
- REST fallback: use `opportunities pipelines` shape.
- Return ambiguity errors with candidate names, not silent wrong matches.

### Task 3.2: Add `activity rollup`

**Command:**

```bash
topline --agent activity rollup \
  --pipeline "Sales - Flex - Qualified" \
  --since this-week-et \
  --group-by channel,direction
```

**Objective:** One command for the exact Discord prompt: “What activity happened this week in our qualified pipeline?”

**Output shape:**

```json
{
  "answerType": "pipeline_activity_rollup",
  "pipeline": { "id": "...", "name": "Sales - Flex - Qualified" },
  "window": { "since": "...", "timezone": "America/New_York" },
  "activity": [
    { "type": "Email", "direction": "outbound", "touches": 5, "contactsTouched": 4 }
  ],
  "movement": {
    "stageMoves": 0,
    "opportunityUpdates": 0
  },
  "evidence": { "source": "warehouse_sql", "sqlTemplates": ["pipeline.activity_by_channel", "pipeline.movement"] },
  "fallbackUsed": false
}
```

### Task 3.3: Add `pipeline snapshot`

**Command:**

```bash
topline --agent pipeline snapshot --pipeline "Sales - Flex - Qualified" --status open
```

**Returns:** open count/value by stage plus warehouse freshness metadata.

### Task 3.4: Add `pipeline movement`

**Command:**

```bash
topline --agent pipeline movement --pipeline "Sales - Flex - Qualified" --since this-week-et
```

**Returns:** created, updated, stage-changed, status-changed, won/lost counts, explicitly all-status when proving negatives.

### Task 3.5: Add REST fallback but mark it clearly

**Rule:** if `query doctor` fails, report commands can call existing `pipeline audit`, but output must include:

```json
"fallbackUsed": true,
"fallbackReason": "TOPLINE_QUERY_TOKEN missing"
```

---

## Phase 4: Agent-Shaped Answer Packets

### Task 4.1: Standardize report packet schema

**Objective:** Make every report easy for Paul/Francis/Bernard to summarize consistently.

**Fields:**

- `toplineAnswer`
- `facts`
- `movement`
- `activity`
- `hygieneFlags`
- `nextActions`
- `evidence`
- `freshness`
- `warnings`
- `rawRows` optional behind `--include-rows`

### Task 4.2: Add freshness metadata

**Objective:** Prevent SQL-vs-live confusion.

**Best path:** expose sync timestamps from os-mcp schema/overview if available. If not currently available, open a companion os-mcp issue/PR to add it to `/query/api/get-overview`.

**CLI behavior:**

- If freshness is present, include it.
- If absent, include `freshness.known=false` and warn that SQL is synced warehouse data.

---

## Phase 5: GitHub Repo Improvements

### Task 5.1: Commit local wrapper/install support to repo

**Objective:** Avoid one-off local wrapper drift.

**Files:**
- Create: `scripts/install-local.sh`
- Modify: `README.md`
- Modify: `docs/examples.md`

**Script responsibilities:**

- Build `topline-bin`.
- Install wrapper to `~/.local/bin/topline`.
- Wrapper allowlist includes all `TOPLINE_*` keys needed by REST and SQL.
- Does not create or print secrets.

### Task 5.2: Add docs for SQL-first reporting commands

**Files:**
- Modify: `README.md`
- Modify: `docs/examples.md`
- Create: `docs/sql-first-reporting.md`

**Docs should include:**

- SQL vs REST decision rule.
- `query doctor`.
- `activity rollup` examples.
- Query token setup without leaking a token.
- Freshness caveat.

### Task 5.3: Update bundled skills after code lands

**Files:**
- Modify: `skills/hermes/SKILL.md`
- Modify: `skills/claude-code/SKILL.md`

**Also sync Hermes active skills:**

- `/Users/oc/.hermes/skills/topline-stack/topline-os-cli`
- `/Users/oc/.hermes/skills/topline-stack/topline-os-crm-audits`
- `/Users/oc/.hermes/profiles/sales_agent/skills/topline-stack`
- `/Users/oc/.hermes/profiles/marketing_agent/skills/topline-stack`

---

## Phase 6: PR Strategy

### PR A: Local readiness and `query doctor`

- Base: `main`
- Branch: `feat/query-doctor`
- Scope: local install script, wrapper allowlist, `query doctor`, docs.
- Tests: `go test ./...`.

### PR B: Query template registry

- Base: `main` after PR A merges.
- Branch: `feat/query-templates`
- Scope: `internal/queries`, `query templates`, `query template`.
- Tests: template rendering, query client httptest.

### PR C: Sales-native report commands

- Base: `main` after PR B merges.
- Branch: `feat/sql-first-reports`
- Scope: `activity rollup`, `pipeline snapshot`, `pipeline movement`, resolver, output packets.
- Tests: golden JSON report outputs.

### PR D: os-mcp freshness/catalog improvements, if needed

- Repo: `/Users/oc/workspace/os-mcp`
- Scope: add sync freshness metadata to `/query/api/get-overview` and MCP `topline_describe_schema` if not already present.
- Tests: Worker/unit tests in os-mcp.

---

## Final Acceptance Test

In a fresh Discord sales thread, ask:

> What activity happened this week in our qualified pipeline?

Expected agent behavior:

1. Load Topline OS skills.
2. Run `topline --agent query doctor` or directly use SQL-ready command if already proven.
3. Run high-level SQL-first command, ideally:

```bash
topline --agent activity rollup --pipeline "Sales - Flex - Qualified" --since this-week-et
```

4. Answer with:

- Activity by channel/direction.
- Number of contacts/opportunities touched.
- Stage/status movement separately.
- Open pipeline value/stage snapshot if requested.
- Freshness/caveat line.
- No REST fan-out unless SQL is unavailable.

## Done Criteria

- Local binary and wrapper are synced and SQL-capable.
- `topline --agent query doctor` gives a clear ready/not-ready result.
- Common CRM questions use high-level report commands instead of bespoke SQL.
- Report outputs are compact, evidence-backed, and Discord-friendly.
- GitHub repo has merged PRs with tests and docs.
- Active Hermes/Paul/Bernard skills guide agents to the new commands.
