# Research: Quality baseline

Each entry is answered by a probe or by an independent measurement at a named revision, never by a tool's name.

## R1 Complexity (Q3)

- `gocognit` v1.3.0 exports `Analyzer`, an `analysis.Analyzer`, read from pkg.go.dev on 2026-11-01; `gocyclo` v0.6.0 exports only a library API, so cyclomatic needs the smallest adapter in this repository.
- Independent advisory measurement at `00a4c6b` over the 53 first-party non-test Go files: cyclomatic maximum 34 and cognitive maximum 40, both `quality/runner/cmd/main.go run`; named debt: `registry.go validateCheck` 27/33, `release.Notes` 21/32, `coverage.Config.Check` 15/31, `runner.Run` 20/29 (`m9-go-complexity-result.json`). The M5 figure of 27 was stale.
- Decision: per-function ceilings at the measured value for the named debt, with an owner each; the new-function ceiling from the tools' documented defaults, verified at landing; a ceiling only falls; the base evaluator refuses a rise. TypeScript measured the same way before any budget.
- TypeScript at T006: the pinned ESLint's `complexity` rule, run at threshold 1 over every first-party TypeScript and JavaScript file at `a287ed7`, reported 51 functions in 21 files with a maximum of 12 (a browser journey in `web/e2e/notes.spec.mjs`) and 11 for `toFailure`, the total mapping of error codes; the ceiling is 12 in the one lint configuration, enforced by the existing lint check at pre-commit and full, with a planted control. Cognitive complexity for TypeScript needs a rule package that is not pinned (`eslint-plugin-sonarjs` is the candidate): a pin for T011, probed then.

## R2 Duplication (Q1)

- Independent advisory run of `dupl` v1.1.0 under Go 1.27.2 at `00a4c6b`: zero candidate pairs at the default 100 AST nodes, input count verified, a planted 150-node pair detected by the same tool (`m9-go-duplication-result.json`).
- `dupl` ignores AST values; a candidate needs semantic confirmation before deletion. TypeScript and Python tools probed at landing.
- At T004: dupl 1.1.0 pinned in `quality/go.mod` (sums from the module proxy, MIT read from the module's LICENSE) and bound as `duplication` through `quality/adapters/duplication.sh` (`-files -plumbing -t 100` over the selected Go files, test files, generated protocol code and fixtures excluded with their reasons); 0 candidate clone lines over the tracked Go files; the witness plants a clone pair of well over a hundred nodes and sees it named. `jscpd` for TypeScript and a Python duplication tool need unpinned installs, so they are pins for T011.

## R3 Dead code (Q2)

- `nogo` runs staticcheck's correctness analyzers and not `U1000`; x/tools `deadcode` is reachability-based from main packages with `-test` for test executables and documented defaults for generated code and marker methods. Entry points to declare: `//bff/cmd/*`, the quality runner and checks, `tools/pins/*.py` generators. Probe at landing.
- Probed at T005: staticcheck's `unused` analyzer is a `lint.Analyzer` whose result is a list of unused objects that staticcheck's own driver reports; `nogo` has no such driver, so `quality/analysis/u1000` is that step (one diagnostic per unused object at its position) and is bound in `nogo`. Zero findings over the tree; a hand-planted unused function was refused by the build. The analysis test driver (`analysistest`) skips silently under Bazel because the go command is not the one the test binary was built with, so the witness loads and analyzes the planted package itself with the pinned SDK, in the root module that declares it. x/tools `deadcode` needs `golang.org/x/telemetry`, which is not pinned, so building it is a pin (T011); `knip` needs an unpinned install, refused here, so it is a pin too. Exported identifiers are reachable by definition in per-package mode; whole-program reachability is what `deadcode` would add.

## R4 Coverage (Q5)

- Go's cover tool reports statements, not branch outcomes, and does not probe inside `&&` and `||`. Root's probe of gobco 1.3.4 under Go 1.27 (`m9-go-branch-probe-result.json`, synthetic fixture, no Bazel integration): the tool has `-branch`; on the fixture it reported Condition 2/2 and Branch 2/2 while native statements were 50% and the `select` cases were never executed. It can be qualified only as a branch sub-metric: the probe covered `if` branches only, and an uncalled `select` is not counted even with `-branch`; it is never a complete Go branch claim and never a waiver. Decision: probe `gobco` with adversarial fixtures and accept only the constructs it reports, named as such; Go branch outcomes stay a blocking owned task until measured; browser coverage through Playwright's V8 coverage into the existing merger; floors unchanged and only rising.

## R5 Property tests (Q6)

- Go property tests exist for the notes app; `fast-check` under rules_js and Hypothesis through the pinned uv for the first-party generators, probed at landing. Invariants named before tests: the generated tables round-trip through the inventory check; the runner's selector is order-independent; the client's error mapping is total.

## R6 Mutation (Q7)

- Gremlins reports uncovered, killed, lived, timed-out and non-viable separately and warns that large modules take hours; Stryker for TypeScript. Probe runtime on `quality/runner` and `web/client` first, with the pristine pass before any kill; survivors inspected and recorded; exclusions with reasons.

## R7 Fakes and types (Q8, Q4)

- The memory store is a working fake without a shared contract suite; one is added and will run against a production adapter when an adopter brings one. Type escape hatches are inventoried from the tree with a justification each.

## R8 The archive primitive (FR-015)

- `git bundle verify` reads only the header; a truncated bundle passes it and fails a fresh clone, so the restore is the proof. The primitive owns a temporary directory, removes nothing unowned, refuses links and existing outputs, computes the digest and mode inside the owned directory before publication, publishes with `ln -T` so a directory or link that arrived at the destination meanwhile is refused rather than entered, and reports nothing unless the published path is the owned bundle; the handoff records eligibility only behind a verified archive. An independent output control found the earlier text entering a late-arriving directory and reporting an archive with an empty digest; that case is now a control. 27 primitive and 9 handoff controls pass externally. They are prevention for the future; they recover nothing, and the 2026-11-01 loss stays unrecovered regardless of what they show.

## R10 Config-driven behavior (Q10)

- The human baseline of 2026-11-01 adds typed, versioned configuration: one schema, defaults once at the composition or input boundary, required inner parameters, precedence, validation, effective-value provenance, OpenFeature only where runtime evaluation fits, and no bypass of security, quality, spend or invariant contracts.
- Where this tree stands: the server's `Config` validates its mode, authenticator, store and origin at the boundary (`bff/server`); the quality runner takes its paths from the registry and the binary's arguments; the checks read policy inputs from files with unknown fields refused. Scattered defaults are not inventoried yet. The probe at landing inventories every default in first-party Go and TypeScript by its site, then decides schema placement by focused changes. No framework is added; a library is probed before a pin like every other tool.
- Decision at T021 (2026-11-01): OpenFeature is not adopted and nothing is pinned, because no behavior in this tree is evaluated at run time: every value is read once at start, from a composition or a typed input file, and a change is a restart. If a runtime-evaluated behavior ever appears, its boundary fallback source is the same input file, and the provider is probed then. The surfaces and their configuration-only witnesses and bypass controls are in [docs/quality/defaults.md](../../docs/quality/defaults.md).

## R11 Python applicability (Q11)

- The tree holds three first-party Python scripts, all generators run through the pinned uv: `tools/pins/gen_browser_libs.py`, `tools/pins/gen_multitool_lock.py`, `tools/release/inventory_tables.py`. The suppression ban already covers Python directives (S007). No Python type checker, linter, complexity or coverage tool is bound; the Go and TypeScript gate texts do not waive them. T022 probes each tool through the uv route before any pin and records applicability per script, with coverage measured as lines and branches separately.

## R12 Adoption matrix

- The reconciliation of the 2026-11-01 portfolio adoption matrix is in [adoption.md](adoption.md): reused controls by witness, bounded tasks T022 to T026, and non-applicability reasons. The matrix itself stays outside the repository.

## R9 The budget comparator and its base

- `floors-from-base` builds the evaluator from the base commit and compares the proposal's coverage floors with it (spec 007 R6). The base `e3b3b34` holds no comparator for complexity or mutation budgets, so a proposal that adds `quality/complexity_budgets.json` would be compared by nothing: the check passes without judging, exactly the gap `docs/quality/capability-states.md` already names for an adapter the proposal pins.
- Decision: the comparator lands in one anchor commit with the first budgets, is reviewed independently at that SHA with its own negative control recorded, and every weaker-proposal witness after it names a base at or after that anchor. Until the anchor is accepted, the direction controls run only as fixtures inside the check's own test, which is advisory for the base path and is recorded as partial, not enforced.
- At T012a: `complexity-from-base` runs the base's own comparator (`checks/complexity:complexity_bin`, built in the base worktree as `floors-from-base` builds its evaluator) against the tested tree's budgets; a base that predates the comparator says it holds none and compares nothing, so the check is visible in every receipt and judges once the named base is at or after the anchor `6436306`. The direction control is enforced from that base onward; at `e3b3b34` it remains partial.
