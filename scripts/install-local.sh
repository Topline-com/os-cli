#!/usr/bin/env bash
# install-local.sh — Build and install the Topline OS CLI for local use.
#
# Produces two files:
#   ~/.local/bin/topline-bin   the compiled Go binary
#   ~/.local/bin/topline       a Hermes-friendly wrapper that loads
#                              TOPLINE_* keys from Hermes .env files
#
# Idempotent: rerun any time to refresh the binary or wrapper.
# Never prints secrets; only the env-var KEYS are written into the wrapper.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
install_dir="${TOPLINE_INSTALL_DIR:-$HOME/.local/bin}"
binary_path="$install_dir/topline-bin"
wrapper_path="$install_dir/topline"
hermes_home="${HERMES_HOME_DEFAULT:-$HOME/.hermes}"

mkdir -p "$install_dir"

echo "==> Building topline-bin from $repo_root"
(cd "$repo_root" && go build -o "$binary_path" ./cmd/topline)
chmod +x "$binary_path"
echo "    installed: $binary_path"

echo "==> Writing wrapper $wrapper_path"
cat > "$wrapper_path" <<WRAPPER
#!/usr/bin/env bash
set -euo pipefail

# Hermes-friendly wrapper for the Topline OS CLI.
# Reads only TOPLINE_* keys from Hermes .env files. Profile .env files may
# contain values that parse for Hermes but are unsafe to \`source\` in bash.
load_topline_env() {
  local env_file="\$1"
  [ -f "\$env_file" ] || return 0

  local key value
  while IFS='=' read -r key value || [ -n "\${key:-}" ]; do
    key="\${key#export }"
    key="\${key%%[[:space:]]*}"
    case "\$key" in
      TOPLINE_PIT|TOPLINE_LOCATION_ID|TOPLINE_BRAND_NAME|TOPLINE_BASE_URL|TOPLINE_QUERY_TOKEN|TOPLINE_QUERY_BASE_URL|TOPLINE_MCP_ACCESS_TOKEN|TOPLINE_MCP_TOKEN)
        value="\${value%\$'\r'}"
        if [[ "\$value" == \"*\" && "\$value" == *\" ]]; then
          value="\${value:1:\${#value}-2}"
        elif [[ "\$value" == \'*\' && "\$value" == *\' ]]; then
          value="\${value:1:\${#value}-2}"
        fi
        if [ -z "\${!key:-}" ]; then
          export "\$key=\$value"
        fi
        ;;
    esac
  done < "\$env_file"
}

if [ -n "\${HERMES_HOME:-}" ]; then
  load_topline_env "\$HERMES_HOME/.env"
fi

if [ -z "\${TOPLINE_PIT:-}" ] || [ -z "\${TOPLINE_LOCATION_ID:-}" ] || [ -z "\${TOPLINE_QUERY_TOKEN:-}" ]; then
  load_topline_env "$hermes_home/.env"
fi

exec "$binary_path" "\$@"
WRAPPER
chmod +x "$wrapper_path"
echo "    installed: $wrapper_path"

echo
echo "Done. Verify with:"
echo "  $wrapper_path --agent query doctor"
echo
echo "If 'topline' is not on PATH, add: export PATH=\"$install_dir:\$PATH\""
