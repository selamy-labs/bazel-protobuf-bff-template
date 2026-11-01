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
| [x] T007 | T005 | CI receipt fields, except the base fields that T015 adds; reconcile the evidence-receipt contract | FR-024, FR-015 | T065 | `runner_test` |
| [x] T008 | T006 | `ciparity`: legs from the registry equal jobs and `needs`; no inline policy; unique artifact names; read-only; no `pull_request_target` | FR-003, FR-004, FR-024, FR-025 | T066 | `parity_test` mutations |
| [x] T009 | T008 | Workflow legs, per-attempt receipts; the rerun guide check against R4 stays with T023 | FR-004, FR-024 | T064, T065 | `parity_test`, `zizmor`, `actionlint` |
| [x] T010 | T009 | Gate fixtures by executing its run block: all success, skipped, cancelled, failed, absent, failed then rerun, partial rerun | FR-005 | T061 | `parity_test` |
| [x] T011 | T002, T003 | Merger in the quality module; `.bazelrc`, `bazelrc.allow`; no C++ or JDK | FR-009 | T063 | `bazelrc-policy`, analysis on this no-toolchain template |
| [x] T012 | T011 | `PlanBazel` verb rule; one invocation per stage; `coverage-floor` after it | FR-006, FR-007 | T063 | Fake Bazel count; every required test under `coverage` |
| [x] T013 | T012 | Reports validated: Go statements from the raw profile; Node test entry and a `transport.ts` test; empty, placeholder, truncated, missing module, missing `.cover`, exit 3 with reports | FR-019–FR-021 | T063 | Named fixtures, real tools |
| [x] T014 | T013 | Evaluator: profiles, measured source sets, enrollment, narrowing table R8, suppressions; `node:coverage` added to S005; real Git delete and add control | FR-016–FR-018 | T060, T063 | Unit, real Git fixtures |
| [x] T015 | T014 | `from_base.sh`, `--base`: one resolution, lock, private output base, handle, every refusal in R6; same-tree positive | FR-011–FR-015 | T062 | Real Git fixtures |
| [x] T016 | T015 | Declared `bff-template.policy-base`, installer, `stage.sh`; first push of a topic branch | FR-012, FR-015a | | `fixtures_test` |
| [ ] T017 | T016 | **Anchor commit**: `floors-from-base` and `coverage-floor` registered, first floors from reconciled measurement; record the exit 11 control and targeted evidence. **Hold for independent targeted review** | FR-011, FR-016 | T062, T063 | Review |
| [x] T018 | T017 | Base through `fresh_clone.sh` and CI; bounded measurement summary in the receipt; `escalate` and digest cases | FR-020, FR-022, FR-023 | T062 | `runner_test`, `policy_base_test`, `parity_test` |
| [ ] T019 | T018 | README, parent SC-001, contracts, hook limits | FR-022 | SC-001 | Doc assertions |
| [ ] T020 | T018 | S005 re-witness against the pinned tools; closes or restates parent T047 | FR-027 | T047 | Suppression fixtures |
| [ ] T021 | T018 | Bypassed-hook commit fails a CI-equivalent run | FR-027 | T070 | `fixtures_test` |
| [x] T022 | T009 | Trigger sentence narrowed | FR-026 | | Doc assertion |
| [ ] T023 | T019–T022 | `docs/quality/evidence.md`: separate claims; gaps for T067, T068, FR-023, SC-007 | US5 | T069 | Review |
| [ ] T024 | T023 | Reconcile capability states, classification, parent ledger; cheap checks | all | T070 | Review |
| [ ] T025 | T024 | Reproduce R6 flows exactly, with negatives | SC-004 | | Logs |
| [ ] T026 | T025 | Freeze; normal and hostile gates with `--base`; measure once; hold for independent acceptance | SC-002 | T070 | Receipts |

Remain open by design: parent T067 and T068, blocked on upstream releases.

## Execution record

- T001, T002: plan `8b3c3c7`, probes `c19e962`, before production code.
- T003: `ee8f29e`, red first through three unit tests.
- T004 to T006: `9266a32`. Without `--shard`, behavior is unchanged.
- T007 to T010, T022: `d8bfa82`. The gate's empty-result pass was found while reading it. No hosted run exists.
- T011, T012: `811bdaa`. Two faults were found by a real `bazel coverage`, not by unit tests: `.bazelrc` parsing consumed
  the filter's backslash, so the quality module was silently uninstrumented, and Bazel calls the report generator with
  other arguments.
- T013, T014: `21aafbc`.
- T015, T016: `f4bb2aa`. Review of `21aafbc` found three defects, reproduced red first: a report that counts no statements
  skipped the floor, the check could not see Bazel's real exit, and a malformed branch count read as a hit. The fixture
  showed that prek gives a new branch a real `--from`, which corrected the plan.
- T017: the commit after `f4bb2aa`. Review of `f4bb2aa` found two more, reproduced red first: the base digest used the
  proposal's definition, and `compare` dropped enrollment. `from_base_test` uses a fake base Bazel and evaluator: it proves custody
  and wiring, not that a compiled base rejects a replaced evaluator. T025 proves that composition with a real base.
