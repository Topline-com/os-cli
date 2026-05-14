# Install Topline OS CLI in Claude Code

End-to-end install for using `topline` inside [Claude Code](https://claude.ai/code).

You will:

1. Install the `topline` binary.
2. Set Topline credentials in your shell.
3. Drop the Claude Code skill into `~/.claude/skills/`.
4. Verify from inside Claude Code.

The whole thing should take ~5 minutes.

---

## TL;DR

```bash
# 1. Install
git clone https://github.com/Topline-com/os-cli.git ~/topline-os-cli
cd ~/topline-os-cli && ./scripts/install-local.sh

# 2. Auth (replace with your values)
cat >> ~/.zshrc <<'EOF'
export TOPLINE_PIT="pit-..."
export TOPLINE_LOCATION_ID="..."
export TOPLINE_BRAND_NAME="Topline OS"
export TOPLINE_QUERY_TOKEN="..."   # from https://os-mcp.topline.com/connect
EOF
source ~/.zshrc

# 3. Install the Claude Code skill
mkdir -p ~/.claude/skills
ln -sf ~/topline-os-cli/skills/claude-code ~/.claude/skills/topline-os-cli

# 4. Verify
topline setup-check
topline --agent query doctor
```

Then in Claude Code, ask: *"Run topline setup-check and tell me what's missing."*
The skill auto-loads when you mention Topline OS, CRM, pipeline audits, or `topline`.

---

## Prerequisites

- **macOS or Linux**. Windows works via WSL.
- **Go 1.21+** if you build from source. Check with `go version`. Install via `brew install go` (macOS) or your distro's package manager.
- **Claude Code** installed and logged in (`claude --version`). Skills live under `~/.claude/skills/`.
- **Topline OS credentials**:
  - `TOPLINE_PIT` — Private Integration Token (starts with `pit-`)
  - `TOPLINE_LOCATION_ID` — your sub-account / location ID
  - `TOPLINE_QUERY_TOKEN` — *required for SQL/analytics commands*. Mint at <https://os-mcp.topline.com/connect> with the same PIT + Location ID. Raw PITs are rejected for SQL by design.

---

## Step 1 — Install the binary

### Option A: Local build with Hermes-friendly wrapper *(recommended)*

```bash
git clone https://github.com/Topline-com/os-cli.git ~/topline-os-cli
cd ~/topline-os-cli
./scripts/install-local.sh
```

This produces:

- `~/.local/bin/topline-bin` — the compiled Go binary
- `~/.local/bin/topline` — a wrapper that reads `TOPLINE_*` keys from `~/.hermes/.env` if Hermes is installed, otherwise falls through to your shell env.

Make sure `~/.local/bin` is on `PATH`:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
which topline   # should print ~/.local/bin/topline
```

### Option B: `go install`

```bash
go install github.com/Topline-com/os-cli/cmd/topline@latest
```

This drops the binary at `$(go env GOPATH)/bin/topline`. Make sure that directory is on `PATH`. No Hermes wrapper — you set env vars directly in your shell.

### Option C: Prebuilt binary

If you have a prebuilt `topline` binary, copy it to `~/.local/bin/topline` and `chmod +x` it.

---

## Step 2 — Set credentials

Add to `~/.zshrc` (or `~/.bashrc`):

```bash
export TOPLINE_PIT="pit-XXXXXXXXXXXXXXXXXXXXXXXX"
export TOPLINE_LOCATION_ID="XXXXXXXXXXXXXXXXXXXXXX"
export TOPLINE_BRAND_NAME="Topline OS"
export TOPLINE_QUERY_TOKEN="signed_connection_token"   # required for SQL analytics
# Optional:
# export TOPLINE_BASE_URL="https://services.leadconnectorhq.com"
# export TOPLINE_QUERY_BASE_URL="https://os-mcp.topline.com"
```

`source ~/.zshrc` and confirm:

```bash
echo "PIT set: ${TOPLINE_PIT:+yes}"
echo "Location set: ${TOPLINE_LOCATION_ID:+yes}"
echo "Query token set: ${TOPLINE_QUERY_TOKEN:+yes}"
```

> **Do not commit these to git.** `topline` never prints token values, and the Hermes wrapper only loads the keys listed above — but secrets in shell rc files still leak via screen-shares and backups. Treat them like SSH keys.

### Alternative: per-project credentials

If you want to scope credentials to a single project (e.g. you work on multiple Topline locations), put them in `<project>/.env` and source that file in your Claude Code workspace shell. The CLI itself does not read `.env` files directly — only the Hermes wrapper does — so for a non-Hermes setup, you must `set -a; source .env; set +a` before launching Claude Code.

---

## Step 3 — Install the Claude Code skill

Claude Code loads skills from `~/.claude/skills/<skill-name>/SKILL.md`. This repo ships the skill at `skills/claude-code/SKILL.md`.

**Recommended — symlink** so the skill stays in sync as the repo updates:

```bash
mkdir -p ~/.claude/skills
ln -sf ~/topline-os-cli/skills/claude-code ~/.claude/skills/topline-os-cli
```

Or **copy** if you prefer a frozen version:

```bash
mkdir -p ~/.claude/skills/topline-os-cli
cp ~/topline-os-cli/skills/claude-code/SKILL.md ~/.claude/skills/topline-os-cli/SKILL.md
```

Verify Claude Code picked it up:

```bash
ls -la ~/.claude/skills/topline-os-cli/
head -5 ~/.claude/skills/topline-os-cli/SKILL.md
```

You should see YAML frontmatter (`name: topline-os-cli`, `description: ...`) at the top of `SKILL.md`. If you don't, Claude Code will not auto-load it — pull the latest repo or open an issue.

---

## Step 4 — Verify

From a fresh terminal:

```bash
topline setup-check
topline --agent query doctor
```

`setup-check` confirms `TOPLINE_PIT` + `TOPLINE_LOCATION_ID` are loaded and the LeadConnector REST API answers.
`query doctor` confirms `TOPLINE_QUERY_TOKEN` is present, not a raw PIT, the hosted warehouse schema is reachable, and the expected analytics tables exist.

Then launch Claude Code in any directory:

```bash
claude
```

Ask: *"Use the topline-os-cli skill to run setup-check and query doctor, then summarize what's healthy."*

If Claude Code uses the `topline` command and returns a clean readiness summary, install is complete.

---

## Troubleshooting

**`command not found: topline`**
→ `~/.local/bin` (or `$(go env GOPATH)/bin`) is not on `PATH`. Add the `export PATH=...` line to `~/.zshrc` and re-source.

**`setup-check` fails with `TOPLINE_PIT not set`**
→ Env vars aren't loaded in the shell that launched Claude Code. Close and reopen your terminal, or `exec $SHELL -l`.

**`query doctor` reports `queryTokenPresent: false`**
→ `TOPLINE_QUERY_TOKEN` is empty. Mint one at <https://os-mcp.topline.com/connect> with your PIT + Location ID.

**`query doctor` reports `rawPitDetected: true`**
→ You pasted the PIT into `TOPLINE_QUERY_TOKEN`. SQL requires a *connection-bound* token from `/connect`, not the raw PIT. Replace it.

**`query doctor` reports missing tables**
→ Warehouse coverage gap in `os-mcp`. File an issue at <https://github.com/Topline-com/os-mcp/issues>. Don't fall back to REST silently — the gap should be fixed upstream.

**Claude Code doesn't auto-load the skill**
→ Confirm `~/.claude/skills/topline-os-cli/SKILL.md` exists and starts with YAML frontmatter (`---\nname: topline-os-cli\n...`). If the file has no frontmatter, you have an old copy — repull the repo.

**Skill loads but Claude runs raw SQL anyway**
→ The skill explicitly bans hand-rolled SQL for standard pipeline audits in favor of `topline --agent query audit`. Remind Claude to follow the 3-call contract in the skill.

---

## Uninstall

```bash
rm -f ~/.local/bin/topline ~/.local/bin/topline-bin
rm -f ~/.claude/skills/topline-os-cli   # or rm -rf if you copied instead of symlinked
# Remove the TOPLINE_* exports from ~/.zshrc
```
