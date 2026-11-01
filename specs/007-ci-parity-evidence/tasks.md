# Tasks: CI Parity, Base-Built Floors and Receipts (M6)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[acceptance-matrix.md](acceptance-matrix.md)

This ledger, the plan and the matrix are committed before any production code. One writer, so nothing runs in
parallel. Each commit names its tasks. A box is ticked only when the named witness passes; a tick is a record, not the
evidence. T017 stops for independent review, and later tasks do not start until it clears.

| Task | Needs | Work | Spec | Parent | Witness |
| --- | --- | --- | --- | --- | --- |
| [x] T001 | M5 | Spec, research, plan, tasks, matrix; recheck upstream directly | all | T060–T070 | Review |
| [x] T002 | T001 | Scratch probes: planted vet defect under `coverage`; subprocess records; reconcile Go report and raw profile by file; Node V8 coverage, then a mature pinned tool; S005 spellings | FR-020a, FR-020b, FR-027 | T063, T047 | Research updated |
| [x] T003 | T001 | `capture` reads stdout only under bounded supervision; stderr in the failure diagnostic; red first | FR-010 | | `runner_test` |
| [x] T004 | T001 | `shard`, `postcondition`; validator: neither, both, unknown leg, postcondition without `after_bazel` | FR-001 | T064, T066 | `runner_test` |
| [x] T005 | T004 | `--shard`, derived obligations, receipts list them; no change without the flag | FR-002 | T064 | `runner_test`, fake Bazel |
| [x] T006 | T005 | Leg column in `required_tests.txt`; leg inventory and sandbox evidence; each required test in one Bazel leg | FR-002, FR-023 | T066 | `verify_body_test` per leg |
| [ ] T007 | T005 | CI receipt fields; reconcile the evidence-receipt contract | FR-024, FR-015 | T065 | `runner_test` |
| [ ] T008 | T006 | `ciparity`: legs from the registry equal jobs and `needs`; no inline policy; unique artifact names; read-only; no `pull_request_target` | FR-003, FR-004, FR-024, FR-025 | T066 | `parity_test` mutations |
| [ ] T009 | T008 | Workflow legs, per-attempt receipts; check the rerun guide against R4 | FR-004, FR-024 | T064, T065 | `parity_test`, `zizmor`, `actionlint` |
| [ ] T010 | T009 | Gate fixtures by executing its run block: all success, skipped, cancelled, failed, absent, failed then rerun, partial rerun | FR-005 | T061 | `parity_test` |
| [ ] T011 | T002, T003 | Merger in the quality module; `.bazelrc`, `bazelrc.allow`; no C++ or JDK | FR-009 | T063 | `bazelrc-policy`, analysis on this no-toolchain template |
| [ ] T012 | T011 | `PlanBazel` verb rule; one invocation per stage; `coverage-floor` after it | FR-006, FR-007 | T063 | Fake Bazel count; every required test under `coverage` |
| [ ] T013 | T012 | Reports validated: Go statements from the raw profile; Node test entry and a `transport.ts` test; empty, placeholder, truncated, missing module, missing `.cover`, exit 3 with reports | FR-019–FR-021 | T063 | Named fixtures, real tools |
| [ ] T014 | T013 | Evaluator: profiles, measured source sets, enrollment, narrowing table R8, suppressions; `node:coverage` added to S005; real Git delete and add control | FR-016–FR-018 | T060, T063 | Unit, real Git fixtures |
| [ ] T015 | T014 | `from_base.sh`, `--base`: one resolution, lock, private output base, handle, every refusal in R6; same-tree positive | FR-011–FR-015 | T062 | Real Git fixtures |
| [ ] T016 | T015 | Declared `bff-template.policy-base`, installer, `stage.sh`; first push of a topic branch | FR-012, FR-015a | | `fixtures_test` |
| [ ] T017 | T016 | **Anchor commit**: `floors-from-base` and `coverage-floor` registered, first floors from reconciled measurement; record the exit 11 control and targeted evidence. **Hold for independent targeted review** | FR-011, FR-016 | T062, T063 | Review |
| [ ] T018 | T017 | Base through `fresh_clone.sh` and CI; `escalate` and digest cases | FR-022, FR-023 | T062 | Real-registry tests |
| [ ] T019 | T018 | README, parent SC-001, contracts, hook limits | FR-022 | SC-001 | Doc assertions |
| [ ] T020 | T018 | S005 re-witness against the pinned tools; closes or restates parent T047 | FR-027 | T047 | Suppression fixtures |
| [ ] T021 | T018 | Bypassed-hook commit fails a CI-equivalent run | FR-027 | T070 | `fixtures_test` |
| [ ] T022 | T009 | Trigger sentence narrowed | FR-026 | | Doc assertion |
| [ ] T023 | T019–T022 | `docs/quality/evidence.md`: separate claims; gaps for T067, T068, FR-023, SC-007 | US5 | T069 | Review |
| [ ] T024 | T023 | Reconcile capability states, classification, parent ledger; cheap checks | all | T070 | Review |
| [ ] T025 | T024 | Reproduce R6 flows exactly, with negatives | SC-004 | | Logs |
| [ ] T026 | T025 | Freeze; normal and hostile gates with `--base`; measure once; hold for independent acceptance | SC-002 | T070 | Receipts |

Remain open by design: parent T067 and T068, blocked on upstream releases.
