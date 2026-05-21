# Quickstart: Compile Workflows with --strict on Public Repos

**Audience**: Fleet operators using `gh-aw-fleet` against public and private repos.
**Date**: 2026-05-17
**Spec**: [spec.md](./spec.md)

This quickstart walks through the operator-visible behavior of the `compile_strict` resolution. It assumes you already have a working `gh-aw-fleet` install, `gh aw` v0.68.3+ locally, and at least one repo in `fleet.local.json`.

## TL;DR

- **Public repos**: workflows compile with `--strict` automatically on the next `deploy` or `upgrade`. You don't have to do anything.
- **Private repos**: workflows compile in default (non-strict) mode. You don't have to do anything.
- **Override either default**: set `"compile_strict": true` or `"compile_strict": false` on the repo in `fleet.local.json`.

## Scenario 1: Public repo, no setup needed (default flow)

You added a public repo to `fleet.local.json` and ran a normal deploy:

```bash
gh-aw-fleet deploy rshade/gh-aw-fleet
```

In the dry-run output, you see a new info line on stderr:

```text
INF compile-strict resolution event=compile_strict_resolved repo=rshade/gh-aw-fleet effective=true source=auto-public
```

That tells you: when you `--apply`, `gh aw compile --strict` will run after `gh aw add` and the resulting PR will contain strict-compiled lock files. If you're okay with that, run `--apply`:

```bash
gh-aw-fleet deploy rshade/gh-aw-fleet --apply
```

The PR that opens contains lock files that pass the public-repo runtime check on first merge — no "Recompile with --strict" footgun.

## Scenario 2: Opt a public repo out of strict mode

You have one public repo where a workflow legitimately can't be made strict (e.g., a one-shot migration that needs `permissions: write-all`):

Edit `fleet.local.json`:

```jsonc
{
  "repos": {
    "rshade/legacy-migration": {
      "profiles": ["default"],
      "compile_strict": false   // opt out of auto-on
    }
  }
}
```

Now `gh-aw-fleet deploy rshade/legacy-migration` logs `source=explicit` and skips the compile step entirely. The visibility lookup is also skipped (FR-008), so this works under tokens that lack repo-metadata read scope.

## Scenario 3: Force strict on a private repo

You have a private repo you want to keep strict-clean in anticipation of going public:

```jsonc
{
  "repos": {
    "rshade/staging-private-repo": {
      "profiles": ["default"],
      "compile_strict": true    // force strict despite private visibility
    }
  }
}
```

`source=explicit` again, compile runs.

## Scenario 4: Onboarding feedback at `add` time

When you add a new repo:

```bash
gh-aw-fleet add rshade/some-new-public-repo
```

You see (on stdout, after the existing add confirmation):

```text
public repo: workflows will be compiled with --strict on next deploy (auto-on; override with "compile_strict": false in fleet.local.json)
```

If the visibility lookup fails (token scope, network), the info line is suppressed but the add still succeeds.

## Troubleshooting

### "gh aw compile --strict failed"

Your compile step aborted. The error message includes a hint and points you at the work-dir clone:

```text
Error: gh aw compile --strict failed (exit 1)

Hint (compile_strict_failed):
  Workflow violates strict-mode validation. Inspect the work-dir clone at
  /tmp/gh-aw-fleet-1234567. To opt this repo out, set "compile_strict": false
  in fleet.local.json for rshade/foo.

Raw compile stderr:
  ... gh aw stderr ...
```

Actions to take:

1. `cd /tmp/gh-aw-fleet-1234567` and inspect `.github/workflows/*.lock.yml` to see what compile produced.
2. Identify the workflow that fails strict (the gh-aw stderr names it).
3. Either fix the workflow (preferred — the strict-mode complaint is usually fixable), or set `"compile_strict": false` for the repo if there's a legitimate reason this repo can't be strict.

### "gh aw is too old"

Your local `gh aw` extension predates `--strict`:

```text
Error: gh aw compile does not support --strict

Hint (gh_aw_too_old):
  Local gh aw version is too old (detected v0.50.0; minimum v0.68.3).
  Upgrade with `gh extension upgrade aw`. To bypass, set "compile_strict": false
  for repos that don't need strict compile.
```

Run:

```bash
gh extension upgrade aw
gh aw --version    # confirm >= v0.68.3
```

Then re-run your deploy with `--work-dir <path>` to resume from the preserved clone.

### "gh aw is missing"

Your local `gh aw` extension isn't installed or is broken:

```text
Error: gh aw compile --help failed

Hint (gh_aw_missing):
  gh aw extension is missing or broken. Install with
  `gh extension install github/gh-aw`. Underlying error: <stderr>.
```

Install or reinstall:

```bash
gh extension install github/gh-aw
```

### "Visibility lookup failed; defaulting to strict ON"

The fleet couldn't determine your repo's visibility:

```text
WRN compile-strict visibility lookup failed; defaulting to strict ON
    event=compile_strict_lookup_failed repo=rshade/foo reason=HTTP 403
```

The deploy proceeds with strict ON (fail-secure). If your repo is private and you don't want strict compile, set `"compile_strict": false` explicitly to bypass the lookup entirely. If your token should have read access and doesn't, fix the token scope.

## CI integration

If you consume `--output json`, three new fields appear on every Deploy/Upgrade result:

```bash
gh-aw-fleet deploy rshade/gh-aw-fleet --output json | jq '.result | {
  compile_strict_applied,
  compile_strict_effective,
  compile_strict_source
}'
```

`compile_strict_effective` is the resolver's verdict regardless of whether
the compile subprocess actually ran. In dry-run that's the "would apply on
`--apply`" signal; in `--apply` it matches `compile_strict_applied` unless
the probe or compile aborted.

Use these to gate on the fail-secure case in your CI:

```bash
# Alert when deploy proceeded with strict ON because visibility lookup failed
gh-aw-fleet deploy rshade/gh-aw-fleet --output json | jq '
  if .result.compile_strict_source == "auto-fallback"
  then error("Visibility lookup failed; investigate token / network")
  else . end
'
```

See [contracts/json-envelope.md](./contracts/json-envelope.md) for the full envelope shape.

## What does NOT change

- The fleet still pins `github/gh-aw` source refs to tagged releases in `fleet.json` profiles — separate from your local `gh aw` CLI version.
- `gh aw` workflow markdown is never rewritten by `gh-aw-fleet`. `compile --strict` only touches `.lock.yml` files.
- All existing deploy / upgrade / sync flows remain valid; you don't need to change your CI today unless you want to gate on the new fields.
- Commit-signing behavior is unchanged. The probe and compile step happen before the existing `git commit` step; signing is the operator's responsibility per the three-turn pattern.
