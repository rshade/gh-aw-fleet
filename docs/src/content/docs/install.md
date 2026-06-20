---
title: Install
description: Install gh-aw-fleet from a release asset, with source-build fallbacks.
---

`gh-aw-fleet` is a local CLI for operators who manage multiple repositories that
use [`gh aw`](https://github.com/github/gh-aw)-compiled agentic workflows.

## Prerequisites

- The `gh` CLI, authenticated with `repo` and `workflow` scopes.
- The `gh aw` extension pinned to the same `github/gh-aw` ref used by your fleet.
  The shipped default profile uses `v0.79.2`.
- Go 1.26.4 or newer only if you install with `go install` or build from source.

Install the matching `gh aw` release explicitly:

```bash
gh extension install github/gh-aw --pin v0.79.2
```

Avoid installing `gh-aw` from `main`; unreleased compiler behavior can break
generated workflows before the fleet pins are ready for it.

## One-liner installer

The recommended path downloads the installer from the latest release assets. The
installer then downloads the matching OS/architecture archive, verifies its
SHA-256 against `checksums.txt`, and installs the binary.

```bash
curl -sSfL \
  https://github.com/rshade/gh-aw-fleet/releases/latest/download/install.sh \
  | bash
```

```powershell
iwr -UseBasicParsing https://github.com/rshade/gh-aw-fleet/releases/latest/download/install.ps1 `
  | iex
```

By default, POSIX installs to `$HOME/.local/bin` and PowerShell installs to
`%LOCALAPPDATA%\gh-aw-fleet\bin`.

## Pin a version or install directory

```bash
VERSION=v0.2.0 INSTALL_DIR=/usr/local/bin \
  bash -c "$(curl -sSfL https://github.com/rshade/gh-aw-fleet/releases/latest/download/install.sh)"
```

PowerShell parameters require invoking the downloaded script as a script block:

```powershell
$installer = [ScriptBlock]::Create(
  (iwr -UseBasicParsing https://github.com/rshade/gh-aw-fleet/releases/latest/download/install.ps1).Content
)
& $installer -Version v0.2.0 -InstallDir "$env:LOCALAPPDATA\gh-aw-fleet\bin" -NoPath
```

If the release asset URL is unavailable for an older release, fetch the fallback
installer from `main`; it still installs a tagged release by default:

```bash
curl -sSfL \
  https://raw.githubusercontent.com/rshade/gh-aw-fleet/main/install.sh \
  | bash
```

```powershell
iwr -UseBasicParsing https://raw.githubusercontent.com/rshade/gh-aw-fleet/main/install.ps1 `
  | iex
```

## Manual release download

Pre-built release archives are published for Linux, macOS, and Windows on amd64
and arm64.

```bash
gh release download --repo rshade/gh-aw-fleet --pattern '*linux_amd64.tar.gz'
tar -xzf gh-aw-fleet_*_linux_amd64.tar.gz
sudo mv gh-aw-fleet /usr/local/bin/
```

See the [latest release](https://github.com/rshade/gh-aw-fleet/releases/latest)
for all archives and checksums.

## Go install

```bash
go install github.com/rshade/gh-aw-fleet@latest
```

This installs into `$(go env GOPATH)/bin`, which may need to be added to your
`PATH`.

## Build from source

```bash
git clone https://github.com/rshade/gh-aw-fleet.git
cd gh-aw-fleet
go build -o gh-aw-fleet .
```

## First sanity check

```bash
gh-aw-fleet list
```

The command prints which configuration files were loaded on stderr:
`fleet.json`, `fleet.local.json`, or both.
