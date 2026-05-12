#!/usr/bin/env bash
set -euo pipefail

# Creates and pushes the public Topline-com/os-cli repo.
# Requires: gh auth login with org repo creation rights.

cd "$(dirname "$0")/.."

git config user.name "Alex Skatell"
git config user.email "alex@topline.com"

gh repo create Topline-com/os-cli \
  --public \
  --description "Printing Press-style agent-native CLI for Topline OS" \
  --source . \
  --remote origin \
  --push

gh repo edit Topline-com/os-cli \
  --add-topic topline-os,crm,cli,mcp,sales-ops,agent-tools

gh repo view Topline-com/os-cli --web
