---
title: Configuration
description: Reference for fleet.json, fleet.local.json, profiles, repos, pins, and billing annotations.
---

`fleet.json` is the declarative desired state for the fleet. It answers three
questions:

- Which workflow profiles exist?
- Which upstream source ref pins each profile?
- Which repositories receive which profiles, extras, exclusions, and overrides?

## File model

`gh-aw-fleet` reads up to two config files from the working directory:

- `fleet.json`: the committed base config. This repository ships a public dogfood
  example that tracks only `rshade/gh-aw-fleet`.
- `fleet.local.json`: a gitignored private overlay for real fleet state, private
  repositories, and personal overrides.

When both files exist they are merged. Local profiles, repos, and defaults add to
or override base entries. When only one file exists, that file is used directly.
The CLI prints the loaded mode on stderr, for example
`(loaded fleet.json + fleet.local.json)`.

The loader also accepts `.hujson` variants before `.json`, so comments and
trailing commas are allowed in fleet config files. Having both
`fleet.hujson` and `fleet.json` for the same base is rejected as ambiguous.

## Top-level shape

```json
{
  "version": 1,
  "defaults": {
    "engine": "copilot"
  },
  "profiles": {
    "default": {
      "description": "Baseline every tracked repo gets.",
      "tier": "standard",
      "sources": {
        "github/gh-aw": { "ref": "v0.79.2" },
        "githubnext/agentics": {
          "ref": "96b9d4c39aa22359c0b38265927eadb31dcf4e2a"
        }
      },
      "workflows": [
        { "name": "audit-workflows", "source": "github/gh-aw" },
        { "name": "ci-doctor", "source": "githubnext/agentics" }
      ]
    }
  },
  "repos": {
    "acme/widgets": {
      "profiles": ["default"],
      "engine": "copilot",
      "extra": [],
      "exclude": [],
      "cost_center": "platform"
    }
  }
}
```

## version

`version` is the on-disk fleet config schema version. The current value is `1`.
It is independent from the JSON output envelope schema used by command output.

## defaults

`defaults` holds fleet-wide defaults used when a repo does not override a value.
The common field is `engine`, such as `copilot`.

## profiles

A profile is a named workflow bundle. Repos opt into profiles through their
`profiles` list.

Profile fields:

- `description`: human-readable purpose and operating notes.
- `tier`: optional advisory cost tier such as `minimal`, `standard`, or
  `premium`. The tool accepts any string and does not enforce behavior from it.
- `sources`: upstream repositories and refs used by workflows in the profile.
- `workflows`: workflow names and the source key each name resolves through.

The built-in `default` profile mirrors `profiles/default.json`. Keep those files
in sync when changing the default profile.

## sources

Each source key maps to a ref:

```json
"sources": {
  "github/gh-aw": { "ref": "v0.79.2" },
  "githubnext/agentics": { "ref": "96b9d4c39aa22359c0b38265927eadb31dcf4e2a" }
}
```

`github/gh-aw` production pins must use tagged releases, not `main`. The
`githubnext/agentics` library may pin to `main` or to a commit until the library
tags releases, but a commit SHA is more repeatable.

Path conventions differ by source:

- `githubnext/agentics` uses workflow names under its implicit `workflows/`
  directory.
- `github/gh-aw` uses `.github/workflows/<name>.md`.

`gh-aw-fleet` owns that resolution and passes the final spec to `gh aw add`.

## workflows

Each workflow entry names a workflow and the source key that provides it:

```json
{ "name": "daily-malicious-code-scan", "source": "githubnext/agentics" }
```

Bumping one source ref in a profile re-pins every workflow in that profile from
that source.

## repos

Repo keys are `owner/name` strings.

Repo fields:

- `profiles`: required list of profile names to apply.
- `engine`: optional per-repo engine override.
- `compile_strict`: optional boolean override for `gh aw compile --strict`.
- `extra`: optional workflows outside any profile.
- `exclude`: optional workflow names to omit from the resolved profile set.
- `overrides`: optional map of workflow name to a custom path, for when the
  upstream path does not match the source's convention.
- `cost_center`: optional free-form billing tag used by
  `gh-aw-fleet consumption --by cost-center`.

`extra` can reference local workflows or upstream sources. Local extras point at
workflow markdown already present in the target repository.

## Compile-strict resolution

`deploy` and `upgrade` run `gh aw compile --strict` on the clone after `gh aw
add` / `gh aw upgrade`, before staging changes. The `compile_strict` field
controls whether that strict compile runs, resolved in this order:

1. **Explicit override** — a `compile_strict` value in `fleet.json` /
   `fleet.local.json` wins; the visibility lookup is skipped.
2. **Auto-detect from visibility** — when `compile_strict` is absent, the tool
   reads the repo's `visibility` via `gh api /repos/<owner>/<repo>`. `public`
   turns strict on; `private`, `internal`, or anything else turns it off.
3. **Fail-secure** — if the visibility lookup fails (HTTP error, network failure,
   malformed JSON), the resolver defaults to strict on and logs one `warn` line.

The `--output json` envelope for `deploy` and `upgrade` reports the outcome in
three fields:

- `compile_strict_applied` (`bool`) — true only when the strict compile ran and
  exited `0` this invocation; always false in dry-run.
- `compile_strict_effective` (`bool`) — the resolver's verdict regardless of
  whether compile ran (the "would apply on `--apply`" signal in dry-run).
- `compile_strict_source` (`string`) — `explicit`, `auto-public`, `auto-private`,
  `auto-fallback`, or `""`.

This is separate from the per-invocation `--strict` security gate (see
[Reconcile workflow](/gh-aw-fleet/reconcile/)): compile-strict validates the generated
GitHub Actions YAML, while the security gate blocks on scanner findings.

## Overlay granularity

Merging is per profile or repo entry, not per field. If `fleet.local.json`
redefines `profiles.default`, that local object replaces the base
`profiles.default` object wholesale. Include optional fields like `tier` in the
local copy if you still want them after the overlay.
