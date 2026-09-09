#!/usr/bin/env bash
# Fails when go.mod's `go` directive and mise.toml's `go` pin disagree.
#
# This is the check that would have caught ax-go raising its own go.mod floor
# to 1.27.1 — without it, that surfaces as an opaque "module requires go >=
# 1.27.1" build error far from its cause.
set -euo pipefail

go_mod=$(awk '/^go /{print $2; exit}' go.mod)
mise_go=$(awk -F'"' '/^go =/{print $2; exit}' mise.toml)

if [ -z "$go_mod" ] || [ -z "$mise_go" ]; then
  echo "could not parse a go version (go.mod='$go_mod' mise.toml='$mise_go')" >&2
  exit 1
fi

if [ "$go_mod" != "$mise_go" ]; then
  echo "toolchain drift: go.mod says $go_mod, mise.toml says $mise_go" >&2
  echo "fix with: go mod edit -go=$mise_go" >&2
  exit 1
fi

echo "toolchain OK: go $go_mod"
