# Implementation Plan: CI Parity, Base-Built Floors and Receipts (M6)

**Branch**: `007-ci-parity-evidence` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

The registry's checks split into three legs and shared postconditions, so CI runs the same policy as
`full`. One Bazel invocation gives test results and coverage. Coverage floors and suppression patterns are judged by
an evaluator built from a caller-chosen base. One writer, small traced commits, no remote and no hosted run.

## Shape

- `quality/proto/registry.proto`, `quality/registry.textproto`: `shard`, `postcondition`, validated. Legs and obligations derived.
- `quality/runner`: `--shard`, `--base`; verb selection in `PlanBazel`; CI receipt fields; leg inventory; stdout-only `capture`.
- `quality/checks/coverage`: pure Go merger for the output and report generator flags, keeping the raw Go profile.
- `quality/checks/floors`: the evaluator: profiles, measured source sets, narrowing, floors, suppression patterns.
- `tools/policy/from_base.sh`: a detached base worktree, a private output base, one `bazel build`. Reuses the push worktree's custody:
  exclusive lock, process handle, no shared server.
- `quality/coverage_floors.json`: under `^quality/`, so it escalates. `.bazelrc` and `bazelrc.allow` gain the `coverage` lines.
- `.github/workflows/ci.yml`, `tools/ci/policy.json`, `quality/checks/ciparity`, `tools/ci/mutations.tsv`.
- `tools/verify.sh`, `tools/bootstrap/fresh_clone.sh`, README, parent SC-001: `--base`.
- `tools/hooks/install.sh`, `tools/hooks/stage.sh`: the declared `bff-template.policy-base` setting.
- `docs/quality/evidence.md`, capability states, classification, contracts.

## Stages

1. Spec, research, plan, tasks and matrix committed. Remaining probes T002, then adjust the plan.
2. Stdout-only capture, red first. Legs, obligations and parity. `full` unchanged until the partition test passes.
3. Workflow legs, receipts, gate fixtures.
4. Coverage verb and merger: measured, reported, no floor.
5. `--base` and the hook setting. **Anchor commit**: evaluator, first floors, `floors-from-base` registered. Record the refusal at the
   anchor. Hold for its own independent targeted review. No source approves itself.
6. Descendant commits use it: base through `fresh_clone.sh` and CI, flows R6 reproduced.
7. Evidence, ledgers, cheap checks, freeze, normal and hostile gates with `--base`, measure once, hold for independent acceptance.

M7 then needs M6's acceptance, and a real separate neutral consumer with a canonical-pin upgrade. M8 needs the R6 re-anchor note, the
explicit first-push CI case, and the provisioning plan offline.

## Constitution and gates

- Upstream contracts stay upstream. The evaluator judges coverage profiles and suppression patterns only. It is not a policy-diff, and it
  adds no issuers, no readiness and no branch-protection change.
- No self-baseline path exists in any form: no flag, default or environment variable.
- Closed throughout: origin, publication, settings, Apps, access, apply, destroy, merge, paid runs.
- Mutants run in exclusive copies. A setup failure earns no credit. Red before code is claimed only where it happened.

## Risks

| Risk | Handling |
| --- | --- |
| `nogo` differs under instrumentation | T002 plants a vet defect under `coverage` before relying on it |
| rules_go's `.cover` location moves at a bump | Pin fixture; a missing profile fails |
| Instrumented data binaries change another tool's output | FR-010, plus 36 of 36 under `coverage` at each pin change |
| Bare `tools/verify.sh` now exits 11 | Message prints the fix; README, SC-001 and `fresh_clone.sh` move together |
| Base worktree inherits tested-tree config | Builds from the base's `.bazelrc` and `MODULE.bazel`; a mutant makes the tested tree's copy break it |
| A second private output base doubles disk and memory | Measure; keep the capacity slot |
| TS coverage cannot be measured natively | A mature tool under the 14-day pin rule, with provenance. `unmeasured` is not an end state |
