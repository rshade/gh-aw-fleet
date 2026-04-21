# Quickstart: Structured Logging

**Feature**: 002-add-zerolog-logging
**Audience**: Developers validating the feature locally before opening a PR; reviewers reproducing the claims.

This is a minimal end-to-end reproduction that exercises every flag combination and the three headline behaviors (default quiet, debug verbose, JSON parseable). Runs in under a minute on a typical dev machine.

---

## Prereqs

- Go 1.25.8 toolchain.
- `gh` CLI with `gh auth status` green (for `list` to actually hit GitHub — optional; `list --help` exercises the flag surface without network).
- `jq` on PATH.
- A local `fleet.local.json` or `fleet.json` — any valid file; `list` does not mutate.
- Repo cloned to `/mnt/c/GitHub/go/src/github.com/rshade/gh-aw` (adjust paths below if elsewhere).

---

## Step 1 — Build

```sh
cd /mnt/c/GitHub/go/src/github.com/rshade/gh-aw
go build -o gh-aw-fleet .
```

Expected: binary at `./gh-aw-fleet`. No warnings.

**Failure mode**: If `go build` fails with `package github.com/rs/zerolog: cannot find module`, the dependency was not added. Run `go get github.com/rs/zerolog@latest && go mod tidy` and retry.

---

## Step 2 — Default invocation: stdout byte-identity (SC-001)

```sh
./gh-aw-fleet list > /tmp/list_default.txt 2> /tmp/list_default.stderr
git stash  # stash the feature branch
git checkout main
go build -o /tmp/gh-aw-fleet-baseline .
/tmp/gh-aw-fleet-baseline list > /tmp/list_baseline.txt
git checkout -  # back to feature branch
git stash pop
diff /tmp/list_baseline.txt /tmp/list_default.txt && echo "stdout IDENTICAL"
```

Expected: `stdout IDENTICAL`. If diff produces output, stdout drifted from the pre-feature baseline — FR-008 / SC-001 regression. The most likely cause is a tabwriter line being accidentally rerouted through the logger.

---

## Step 3 — Default invocation: stderr diagnostics check

```sh
cat /tmp/list_default.stderr
```

Expected: the pre-feature stderr content only (`(loaded fleet.local.json)` breadcrumb, etc.). At `info` level with `list` (which has no warnings on the happy path), there should be no new stderr lines introduced by this feature.

---

## Step 4 — Debug level exposes subprocess summaries

```sh
./gh-aw-fleet list --log-level=debug 2> /tmp/list_debug.stderr
grep -c '"level":"debug"\|DBG' /tmp/list_debug.stderr || true
```

Expected: at least one line tagged `DBG` (console format) with `tool=`/`subcommand=`/`exit_code=`/`duration=` fields. `list` may or may not invoke a subprocess depending on caching — if it doesn't, run `deploy --apply=false <repo>` or `template fetch` instead to see summaries.

---

## Step 5 — JSON format parses cleanly (FR-005, SC-002)

```sh
./gh-aw-fleet list --log-format=json 2> /tmp/list_json.stderr
cat /tmp/list_json.stderr | while read -r line; do
  [ -z "$line" ] && continue
  echo "$line" | jq -e '.level, .time, .message' > /dev/null || { echo "NOT JSON: $line"; exit 1; }
done && echo "all stderr lines parse as JSON"
```

Expected: `all stderr lines parse as JSON`. If any line fails, check whether it's a cobra flag-error line (those are plain text by design, per Q4 — but they should only appear when a flag is actually invalid).

---

## Step 6 — Invalid flag rejection (FR-004)

```sh
./gh-aw-fleet list --log-level=shouting 2> /tmp/invalid.stderr; echo "exit=$?"
cat /tmp/invalid.stderr
```

Expected:
- Exit status non-zero (`exit=1`).
- Stderr contains `Error:` (cobra's standard prefix) identifying `--log-level` and the invalid value `shouting`.
- Stderr content is plain text, NOT JSON (Q4 clarification).
- `list` subcommand's own output (e.g., the table of fleet repos) is absent — the subcommand did not run.

Same for `--log-format=yaml` (invalid):
```sh
./gh-aw-fleet list --log-format=yaml 2>&1; echo "exit=$?"
```

---

## Step 7 — Warning routing (FR-009, FR-017) — requires a warning-triggering scenario

Easiest: trigger a `deploy` dry-run against a repo whose target profile references a secret that is not set. Without a live fleet, stub by invoking a code path that calls `log.Warn().Str("repo", ...).Msg(...)` directly from a unit test.

```sh
go test -run TestWarningWithRepoField -v ./cmd/...
```

Expected: test passes. The test asserts that a `warn` event emitted inside subcommand execution reaches stderr as JSON when `--log-format=json` is passed, with `repo` present as a distinct field.

## Step 7b — Single-filter repo isolation (SC-003)

Simulate a multi-repo run and verify one structured filter isolates events for a single repo:

```sh
# Emit a synthetic multi-repo stream (once the logger is wired, any deploy against ≥2 repos
# will do; in a sandbox, a small Go harness emitting 2 warn events with different repo fields
# is equivalent). Output shown for illustration:
cat <<'EOF' > /tmp/mock.json
{"level":"warn","time":"2026-04-20T14:30:22-07:00","repo":"acme/api","message":"x"}
{"level":"warn","time":"2026-04-20T14:30:22-07:00","repo":"acme/web","message":"y"}
{"level":"warn","time":"2026-04-20T14:30:22-07:00","repo":"acme/api","message":"z"}
EOF

jq -c 'select(.repo == "acme/api")' /tmp/mock.json
```

Expected: exactly two lines emitted — both with `repo=="acme/api"`, none with `repo=="acme/web"`. One jq predicate, no multi-line parsing, no regex. This satisfies SC-003's "one structured filter" invariant.

---

## Step 8 — Full CI gate

```sh
make ci
```

Expected: `fmt-check`, `vet`, `lint`, `test` all green. This is the gate the spec names in SC-005 ("`make ci` passes"). Do NOT claim the feature complete until this passes locally — per the project CLAUDE.md rule.

---

## Step 9 — Transitive dependency check (SC-006, Constitution Principle I)

```sh
git checkout main
go mod graph | sort > /tmp/deps_before.txt
git checkout -
go mod graph | sort > /tmp/deps_after.txt
diff /tmp/deps_before.txt /tmp/deps_after.txt
```

Expected: the diff shows exactly one added line — `github.com/rshade/gh-aw-fleet <space> github.com/rs/zerolog@<tag>`. No other additions. If a second dependency appears, the constraint is violated; back out or re-evaluate.

---

## Troubleshooting

**`panic: invalid --log-level "debug": unknown level string`** — zerolog's `ParseLevel` takes lowercase. Confirm the flag value is lowercase; `DEBUG` is rejected.

**JSON output has millisecond-precision timestamps (`.000`) but no timezone** — zerolog default. Ensure `zerolog.TimeFieldFormat = time.RFC3339` is set in `internal/log.Configure` before logger construction.

**Subprocess summary never appears at debug** — check that call sites inside `internal/fleet/*.go` route through `runLogged` rather than bare `cmd.Run()`. A bare `Run()` call is a regression.

**Warning still appears on stdout** — it wasn't migrated. Search `cmd/deploy.go` and `cmd/sync.go` for `⚠` and confirm each occurrence was replaced with a `log.Warn()` call.
