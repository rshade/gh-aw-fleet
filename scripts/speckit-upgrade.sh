#!/usr/bin/env bash
# Upgrades every Spec Kit file in the repo to the specify-cli version pinned in
# mise.toml: the shared .specify/ infrastructure, each integration registered
# in .specify/integration.json, and each extension in .specify/extensions.
# Spec Kit registers extension commands for the default integration only.
#
# Managed files edited in this repo (plan-template.md carries the project's
# Documentation Impact gate) are preserved unless FORCE=1, which overwrites
# them and reinstalls everything.
#
# Bash 3.2 compatible (macOS /usr/bin/env bash): no mapfile, no arrays.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

command -v mise >/dev/null 2>&1 || {
  echo "mise not found. Install: https://mise.jdx.dev/installing-mise.html" >&2
  exit 1
}

specify() { mise exec -- specify "$@" </dev/null; }
jq() { mise exec -- jq "$@"; }

force_flag=""
if [ "${FORCE:-0}" = "1" ]; then
  force_flag="--force"
fi

registry=.specify/extensions/.registry
# Written before any extension re-add and removed once all succeed. A failed
# `extension add --force` has already removed the extension from both the
# registry and extensions.yml, so this file is the only record that it existed.
pending=.specify/.speckit-upgrade-pending

manifest_version() {
  jq -r '.version' ".specify/integrations/$1.manifest.json" 2>/dev/null || true
}

bundled_version() {
  local manifest
  manifest=$(find "$(mise where pipx:specify-cli)" \
    -path "*/core_pack/extensions/$1/extension.yml" 2>/dev/null | head -1)
  [ -n "$manifest" ] || return 0
  awk -F'"' '/^  version:/ { print $2; exit }' "$manifest"
}

cli_version=$(mise exec -- specify --version | awk '{ print $2 }')

integrations=$(jq -r '.installed_integrations | join(" ")' .specify/integration.json)
if [ -z "$integrations" ]; then
  echo "no integrations registered in .specify/integration.json" >&2
  exit 1
fi

# Every upgrade rewrites install timestamps, so skip anything already at the
# pin; otherwise a no-op run dirties the manifests.
for key in $integrations; do
  if [ -z "$force_flag" ] && [ "$(manifest_version "$key")" = "$cli_version" ] &&
    [ "$(manifest_version speckit)" = "$cli_version" ]; then
    echo "integration $key: already at specify $cli_version"
    continue
  fi
  echo "integration $key: upgrading to specify $cli_version${force_flag:+ (forced)}"
  specify integration upgrade "$key" ${force_flag:+"$force_flag"}
  # `integration upgrade` exits 0 with "Nothing to upgrade" when the manifest
  # is missing, so confirm the upgrade actually landed.
  if [ "$(manifest_version "$key")" != "$cli_version" ]; then
    echo "integration $key: still not at $cli_version after upgrade;" \
      "run: mise exec -- specify integration install $key" >&2
    exit 1
  fi
done

# Extensions bundled with the CLI ("local" source) are re-added when their
# installed version differs from the CLI's bundled copy, which keeps them on
# the pin offline; `extension update` never moves bundled extensions.
# Catalog-sourced extensions update from their catalog. `extension update` has
# no --yes flag, so its prompt is fed from `yes`.
#
# `extension add --force` reinstalls with priority 10 and enabled, so each
# entry carries the operator's priority and enablement, and every run
# reconciles the registry back to them. Entry format: id=source=priority=enabled.
# Per-hook edits in extensions.yml are not preserved: the re-add rebuilds that
# extension's hook entries from its manifest.
extension_entries() {
  jq -r '.extensions | to_entries[]
    | [.key,
       (.value.source | if type == "string" then . else "catalog" end),
       (.value.priority // 10), (.value.enabled != false)]
    | map(tostring) | join("=")' "$registry" 2>/dev/null || true
}

registry_field() {
  jq -r --arg id "$1" ".extensions[\$id] | if . == null then \"\" else $2 end" "$registry"
}

extensions=$(extension_entries)
if [ -f "$pending" ]; then
  echo "resuming an interrupted extension upgrade ($pending)"
  # Pending entries come first: they hold the state from before the
  # interrupted run, which a half-finished re-add may have reset.
  extensions=$(printf '%s\n%s\n' "$(cat "$pending")" "$extensions" |
    awk -F= 'NF && !seen[$1]++')
fi
printf '%s\n' "$extensions" >"$pending"

for entry in $extensions; do
  id=${entry%%=*}
  rest=${entry#*=}
  source=${rest%%=*}
  rest=${rest#*=}
  priority=${rest%%=*}
  enabled=${rest#*=}

  if [ "$source" != "local" ]; then
    mise exec -- specify extension update "$id" < <(yes)
  else
    installed=$(registry_field "$id" '.version')
    bundled=$(bundled_version "$id")
    if [ -z "$bundled" ]; then
      echo "extension $id: not bundled with specify $cli_version; left as is"
    elif [ -z "$force_flag" ] && [ "$installed" = "$bundled" ]; then
      echo "extension $id: already at bundled $bundled"
    else
      echo "extension $id: ${installed:-missing} -> bundled $bundled"
      specify extension add "$id" --force --priority "$priority"
    fi
  fi

  [ -n "$(registry_field "$id" '.version')" ] || continue
  if [ "$(registry_field "$id" '.priority // 10')" != "$priority" ]; then
    echo "extension $id: restoring priority $priority"
    specify extension set-priority "$id" "$priority"
  fi
  if [ "$(registry_field "$id" '.enabled != false')" != "$enabled" ]; then
    if [ "$enabled" = "false" ]; then
      echo "extension $id: restoring disabled"
      specify extension disable "$id"
    else
      echo "extension $id: restoring enabled"
      specify extension enable "$id"
    fi
  fi
done
rm -f "$pending"

# Report only: copilot is not declared multi-install safe, so status is
# ERROR (exit 1) on every run while it is registered beside codex and claude.
specify integration status || true
