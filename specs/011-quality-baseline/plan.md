# Implementation Plan: Quality baseline

**Branch**: `011-quality-baseline` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

One tracked control table, measured baselines before any policy, budgets that only ratchet toward strictness,
gates bound in the one registry with planted defects, and disjoint slices with one integrator. No second quality
framework: every tool joins through the existing analyzer, lint, coverage and registry routes.

## Technical Context

**Language/Version**: Go 1.27.2, TypeScript 6.0.3, Python 3.12.15 through the pinned uv, Bazel 9.3.0
**Primary Dependencies**: to be pinned only after each probe: gocognit (v1.3.0 measured), an adapter over gocyclo, dupl (v1.1.0 measured), staticcheck `U1000`, x/tools `deadcode`, `knip`, ESLint `complexity` and a cognitive rule, `jscpd`, `gobco`, Playwright coverage, `fast-check`, Hypothesis, a Go mutation tool, Stryker
**Testing**: every gate has a planted defect and a restored control as a required test; measurements are recorded with the revision
**Target Platform**: the supported Linux profile
**Constraints**: no C++ toolchain; no network in tests; no host interpreter in a verdict path; floors and ceilings never move toward leniency
**Open at planning**: each tool's compatibility, answered by its probe in research.md; the new-function ceilings, answered by the tools' documented defaults verified at landing
**Base dependency**: the budget file is a policy format the accepted base `e3b3b34` cannot judge. The comparator for it lands first as its own anchor commit, reviewed independently with its refusal recorded, as M6 did for `floors-from-base` (spec 007 R6); only then can a weaker-proposal witness name a base that holds the comparator. No source approves itself, and the base is never read from the proposal

## Constitution Check

| Principle | Result |
| --- | --- |
| IV. One policy, thin adapters | Every gate is a registry check run by the one runner; adapters translate invocation only |
| V. Policy cannot weaken itself | Budgets are policy inputs the base evaluator compares, with the direction stated per metric; the evaluator's own limit stays stated |
| III. Pinned | Every tool by checksum through the existing locks, with provenance rows |
| Governance: no live action | None |

## Project Structure

```text
docs/quality/baseline.md                 the control table
docs/quality/type-escapes.md             escape hatches with justifications
quality/complexity_budgets.json          budgets, a policy input
quality/analysis/complexity/             gocognit binding, the gocyclo adapter, the Go baseline
quality/checks/duplication/              dupl binding and baseline
quality/analysis/unused/                 U1000 and deadcode entry points
quality/checks/coverage/branches/        the gobco probe and, if accepted, the branch merger
quality/mutation/                        the Go mutation configuration and survivor record
web/lint/{complexity,duplication,unused,escapes}/
web/mutation/                            Stryker configuration and survivor record
web/client/*.property.test.ts, quality/runner/*_property_test.go, tools/release/*_property_test.py
bff/ports/contract_test.go               the fake's contract suite
tools/release/archive_ref.sh, archive_handoff.sh and their tests
specs/011-quality-baseline/
```

## Steps

1. Spec files; the control table at its measured state; the escape-hatch inventory.
2. Probes, one per tool, on disjoint files, from the frozen base; each records compatibility, the measurement and the planted-pair or planted-defect check that proves the measurement is not an empty run.
3. Integrator: pins, provenance rows, bindings, budgets from the measurements; the budget comparator as its own reviewed anchor, then the direction controls against a base at or after it.
4. Deletions of confirmed duplicates and dead code, one concern per commit.
5. The archive primitive and its handoff as required tests.
6. Evidence, statuses, the full verifier, freeze, independent review at the exact revision.
7. Closure only when every applicable control is enforced and evidenced.

## Ownership

| Role | Files |
| --- | --- |
| Integrator (the author) | `BUILD.bazel`, `MODULE.bazel` and its lock, `package.json` and its lock, `quality/registry.textproto`, `quality/rules.textproto`, `web/lint/eslint.config.mjs`, `quality/complexity_budgets.json`, `specs/`, `docs/` |
| A slice's worker | only that slice's new files in the structure above |

## Gates

| Gate | Where | What it refuses |
| --- | --- | --- |
| Measurement before policy | T004 to T010 before T012 | a budget with no measurement older than itself |
| Probe before pin | T011 | a tool pinned by name, or one that cannot be pinned by checksum |
| Planted defect before enforced | T013, `docs/quality/baseline.md` | a row marked enforced without a named failing gate |
| Direction per metric | T013 | a ceiling that rises, a floor that falls, a blanket exclusion |
| Comparator anchor before witness | T012a before T013 | a weaker-proposal control whose base predates the comparator, which would pass without judging |
| Disjoint slices | T015 | a file touched by two slices |
| Archive before any Git maintenance | T016 | cleanup eligibility without a verified archive; any cleanup step inside the primitive or its handoff |
| Closure | T018 | an applicable control still partial or a gap |

## Risks

- A tool runs but measures nothing: every measurement checks its input count and a planted defect, and an empty report is an error.
- Mutation runtime on the policy packages is unknown: the probe records it first, and the scope is narrowed by measurement, not by exclusion without a reason.
- Go branch outcomes are not produced by the toolchain: the gap stays a blocking owned task; a condition-coverage tool is named by its constructs and gaps, never as branch coverage.
- A late writer at an output path: the archive primitive reads its digest inside the owned directory, publishes by naming the destination itself, and reports only when the published path is the owned bundle; an independent output control found the earlier text entering an arrived directory, and that case is now a control.

## Complexity Tracking

| Exception | Why | Owner, condition |
| --- | --- | --- |
| Go branch outcomes are not produced by the toolchain | the cover tool reports statements and does not probe inside `&&` and `\|\|` | Q5: a condition-coverage tool accepted only for what the probe shows, named by its construct and its `select` gap; the gap stays an owned blocking task until measured |
