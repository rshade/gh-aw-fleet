# Phase 0 Research: Over-Budget Highlighting

All open questions from the spec and the source issue (#129) are resolved below. No
`NEEDS CLARIFICATION` markers remain in the Technical Context.

## Decision 1 — Threshold unit: AIC (native), not derived USD

**Decision**: The `--budget` ceiling is expressed in AI credits (AIC), compared against
each row's `AIC` field.

**Rationale**: AIC is the authoritative billing unit under the Copilot model; USD is a
flat derivation (`aicToUSD` in `internal/fleet/consumption_logs.go`, rate `0.01`). The
rollup already ranks the top-burners footer on AIC/cost, and the `AIC` column is the
primary spend signal. A USD ceiling would just be `budget / aicToUSDRate` and adds a
conversion surface for no behavioral gain. The issue explicitly recommends AIC.

**Alternatives considered**: USD ceiling (`--budget` in dollars) — rejected as a redundant
re-expression of the same number; deferred as a possible future `--budget-usd` if ever asked.

## Decision 2 — Single global ceiling, not a per-axis map

**Decision**: One global ceiling applied uniformly to every row of the active `--by` axis.

**Rationale**: Per-key ceilings (e.g., a different ceiling per cost-center) multiply the
flag surface and the validation/config story with no demonstrated demand. The issue
recommends starting with a single global threshold. A per-axis map remains a clean future
extension (it would read ceilings from `fleet.json`, a separate config-contract change).

**Alternatives considered**: Per-axis threshold map keyed by group key — deferred (out of scope).

## Decision 3 — Comparison semantics: strictly greater than

**Decision**: A row is over budget iff its `AIC` is non-nil **and** `*AIC > budget`. Equal
to the ceiling is within budget. A nil `AIC` (the all-or-nothing nil-merge case) is never
over budget.

**Rationale**: "Over budget" reads in plain language as *exceeding* the ceiling, not
*reaching* it. The nil exclusion is forced by correctness — "exceeds an unknown value" is
undefined, and `mergeFloat`'s all-or-nothing rule already yields nil for any group with a
missing contributor (e.g., a repo whose runs all failed). Flagging nil would be a false
positive and would also require a tri-state, which the boolean field deliberately avoids.

**Alternatives considered**: `>=` (greater-or-equal) — rejected as surprising at the exact
ceiling. Treating nil as over budget — rejected as a false positive.

## Decision 4 — Flag plumbing: `Float64Var` + `Changed()` supplied-detection

**Decision**: Declare `--budget` as a `cmd.Flags().Float64Var(&flags.budget, "budget", 0, ...)`
and detect supplied-ness with `cmd.Flags().Changed("budget")`. When supplied, build a
`*float64` and pass it to `ApplyBudget`; when not, pass `nil` (no annotation).

**Rationale**: `0` is a *valid* ceiling ("flag every row with positive spend"), so the
zero-value default cannot double as "unset". `Changed()` is cobra's idiomatic absent-vs-zero
discriminator and mirrors the codebase's `*float64` "absent vs present-and-zero" pattern
(`awInfoPayload.Cost`). Avoids a sentinel like `-1`.

**Alternatives considered**: `Float64` flag with a `-1` sentinel — rejected as a magic value
that collides with the negative-input validation path. `StringVar` parsed manually —
rejected; `Float64Var` gives cobra-native parse errors for free, but see Decision 5 on
where validation messages are surfaced.

## Decision 5 — Input validation: negative rejected; non-numeric handled by cobra; zero accepted

**Decision**: A negative ceiling is rejected in `runConsumption` with a clear error and a
non-zero exit (and, in `--output json` mode, via `preResultFailureEnvelope`). A
non-numeric value is rejected by cobra's `Float64Var` parser before `RunE` runs (standard
cobra flag-parse error, non-zero exit). A zero ceiling is accepted and flags every row with
strictly-positive AIC.

**Rationale**: FR-011/FR-012 — malformed input is the *only* non-zero exit; it is categorically
distinct from a budget breach, which never changes the exit code. Validating negativity in
`runConsumption` (after `Changed()` detection, before/independent of aggregation) mirrors the
existing `ParseGroupBy`/`buildFetchMode` validation placement and its jsonMode branching.

**Alternatives considered**: Accepting negative as "flag everything" — rejected as nonsense
input that almost certainly indicates operator error; an explicit error is friendlier.

## Decision 6 — Apply point: pure post-aggregation pass `ApplyBudget`

**Decision**: Add an exported `ApplyBudget(res *ConsumptionResult, budget *float64)` in
`internal/fleet/consumption_budget.go` that, when `budget != nil`, sets `res.Budget = budget`,
marks each `res.Groups[i].OverBudget` and each `res.TopBurners[i].OverBudget` by the
Decision-3 rule, and is a no-op when `budget == nil`. `AggregateConsumption` is unchanged;
`runConsumption` calls `ApplyBudget` on its result before rendering/enveloping.

**Rationale**: FR-013 demands a pure function of the already-aggregated result — no new
fetch, fully offline-testable. Keeping it out of `AggregateConsumption` preserves that
function's signature and its network-bound test surface, and gives a tiny stateless unit
under test. Annotating `TopBurners` (always workflow-keyed) as well as `Groups` satisfies
the spec edge case that the footer not disagree with the body for a workflow appearing in both.

**Alternatives considered**: Threading `budget` into `AggregateConsumption` — rejected; it
would entangle the pure check with the network-bound aggregator and complicate its tests.
A `cmd`-layer-only annotation — rejected; the JSON envelope (built in `cmd` from the
`*ConsumptionResult`) needs the per-group boolean to be on the shared struct.

## Decision 7 — Output surfaces: additive only

**Decision**:
- JSON: add `ConsumptionResult.Budget *float64 \`json:"budget,omitempty"\``,
  `ConsumptionGroup.OverBudget *bool \`json:"over_budget,omitempty"\``, and
  `WorkflowConsumption.OverBudget *bool \`json:"over_budget,omitempty"\``. `cmd.SchemaVersion` stays `1`.
- Text: `renderConsumptionText` adds a trailing `OVER` column (value `!` for over-budget
  rows, empty otherwise) to both the primary table and the top-burners footer — **only when
  `res.Budget != nil`**. With no budget, output is byte-identical to today.

**Rationale**: FR-006/FR-008 — opt-in, additive, no schema bump. `budget` and
`over_budget` use `omitempty` so the envelope is unchanged when no ceiling is supplied.
When a ceiling is supplied, `over_budget` is present on every row as true or false for
predictable consumer parsing. A trailing column avoids disturbing existing column
positions/alignment.

**Alternatives considered**: Re-sorting over-budget rows to the top — deferred (spec
Assumption: annotate in place; the rollup already orders by spend). A colorized marker —
rejected; the tool emits plain tabwriter text, and color would not survive piping/JSON.

## Decision 8 — Reconciliation decision record (FR-014)

**Decision**: Add `specs/009-consumption-subcommand/decisions/0001-highlight-not-alarm.md`
(a lightweight ADR) recording that read-only highlighting of operator-pulled output is
distinct from alarming/enforcing, and that `--budget` stays on the highlight side of
009's FR-023. Bundled in this feature's PR, not a separate blocking prerequisite.

**Rationale**: The record's content is fully determined by this feature's read-only design,
so there is nothing to learn by sequencing it first. A `decisions/` subfolder is the
conventional home and keeps the amendment adjacent to the requirement it qualifies without
rewriting 009's ratified spec text.

**Alternatives considered**: Editing FR-023 in place — rejected; the requirement still holds
(no enforcement), so amending its text would misrepresent history; an additive ADR is honest.
