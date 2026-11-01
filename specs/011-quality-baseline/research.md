# Research: Quality baseline

Each entry is answered by a probe or by an independent measurement at a named revision, never by a tool's name.

## R1 Complexity (Q3)

- `gocognit` v1.3.0 exports `Analyzer`, an `analysis.Analyzer`, read from pkg.go.dev on 2026-11-01; `gocyclo` v0.6.0 exports only a library API, so cyclomatic needs the smallest adapter in this repository.
- Independent advisory measurement at `00a4c6b` over the 53 first-party non-test Go files: cyclomatic maximum 34 and cognitive maximum 40, both `quality/runner/cmd/main.go run`; named debt: `registry.go validateCheck` 27/33, `release.Notes` 21/32, `coverage.Config.Check` 15/31, `runner.Run` 20/29 (`m9-go-complexity-result.json`). The M5 figure of 27 was stale.
- Decision: per-function ceilings at the measured value for the named debt, with an owner each; the new-function ceiling from the tools' documented defaults, verified at landing; a ceiling only falls; the base evaluator refuses a rise. TypeScript measured the same way before any budget.

## R2 Duplication (Q1)

- Independent advisory run of `dupl` v1.1.0 under Go 1.27.2 at `00a4c6b`: zero candidate pairs at the default 100 AST nodes, input count verified, a planted 150-node pair detected by the same tool (`m9-go-duplication-result.json`).
- `dupl` ignores AST values; a candidate needs semantic confirmation before deletion. TypeScript and Python tools probed at landing.

## R3 Dead code (Q2)

- `nogo` runs staticcheck's correctness analyzers and not `U1000`; x/tools `deadcode` is reachability-based from main packages with `-test` for test executables and documented defaults for generated code and marker methods. Entry points to declare: `//bff/cmd/*`, the quality runner and checks, `tools/pins/*.py` generators. Probe at landing.

## R4 Coverage (Q5)

- Go's cover tool reports statements, not branch outcomes, and does not probe inside `&&` and `||`; `gobco` reports condition coverage with an explicit `select` gap. Decision: probe `gobco` with adversarial fixtures and accept only the constructs it reports, named as such; Go branch outcomes stay a blocking owned task until measured; browser coverage through Playwright's V8 coverage into the existing merger; floors unchanged and only rising.

## R5 Property tests (Q6)

- Go property tests exist for the notes app; `fast-check` under rules_js and Hypothesis through the pinned uv for the first-party generators, probed at landing. Invariants named before tests: the generated tables round-trip through the inventory check; the runner's selector is order-independent; the client's error mapping is total.

## R6 Mutation (Q7)

- Gremlins reports uncovered, killed, lived, timed-out and non-viable separately and warns that large modules take hours; Stryker for TypeScript. Probe runtime on `quality/runner` and `web/client` first, with the pristine pass before any kill; survivors inspected and recorded; exclusions with reasons.

## R7 Fakes and types (Q8, Q4)

- The memory store is a working fake without a shared contract suite; one is added and will run against a production adapter when an adopter brings one. Type escape hatches are inventoried from the tree with a justification each.

## R8 The archive primitive (FR-015)

- `git bundle verify` reads only the header; a truncated bundle passes it and fails a fresh clone, so the restore is the proof. The primitive owns a temporary directory, removes nothing unowned, refuses links and existing outputs, computes the digest and mode inside the owned directory before publication, publishes with `ln -T` so a directory or link that arrived at the destination meanwhile is refused rather than entered, and reports nothing unless the published path is the owned bundle; the handoff records eligibility only behind a verified archive. An independent output control found the earlier text entering a late-arriving directory and reporting an archive with an empty digest; that case is now a control. 27 primitive and 9 handoff controls pass externally; the 2026-11-01 loss stays unrecovered.

## R9 The budget comparator and its base

- `floors-from-base` builds the evaluator from the base commit and compares the proposal's coverage floors with it (spec 007 R6). The base `e3b3b34` holds no comparator for complexity or mutation budgets, so a proposal that adds `quality/complexity_budgets.json` would be compared by nothing: the check passes without judging, exactly the gap `docs/quality/capability-states.md` already names for an adapter the proposal pins.
- Decision: the comparator lands in one anchor commit with the first budgets, is reviewed independently at that SHA with its own negative control recorded, and every weaker-proposal witness after it names a base at or after that anchor. Until the anchor is accepted, the direction controls run only as fixtures inside the check's own test, which is advisory for the base path and is recorded as partial, not enforced.
