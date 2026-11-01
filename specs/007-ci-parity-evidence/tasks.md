# Tasks: CI Parity, Base-Built Floors and Receipts (M6)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[acceptance-matrix.md](acceptance-matrix.md)

This ledger, the plan and the matrix are committed before any production code. One writer, so nothing runs in
parallel. Each commit names its tasks. A box is ticked only when the named witness passes; a tick is a record, not the
evidence. T017 stops for independent review, and later tasks do not start until it clears.

- [x] T001 Spec, research, plan, tasks, matrix; recheck upstream directly. **Needs** M5. **Spec** all. **Parent** T060–T070. **Witness** Review
- [x] T002 Scratch probes: planted vet defect under `coverage`; subprocess records; reconcile Go report and raw profile by file; Node V8 coverage, then a mature pinned tool; S005 spellings. **Needs** T001. **Spec** FR-020a, FR-020b, FR-027. **Parent** T063, T047. **Witness** Research updated
- [x] T003 `capture` reads stdout only under bounded supervision; stderr in the failure diagnostic; red first. **Needs** T001. **Spec** FR-010. **Parent** . **Witness** `runner_test`
- [x] T004 `shard`, `postcondition`; validator: neither, both, unknown leg, postcondition without `after_bazel`. **Needs** T001. **Spec** FR-001. **Parent** T064, T066. **Witness** `runner_test`
- [x] T005 `--shard`, derived obligations, receipts list them; no change without the flag. **Needs** T004. **Spec** FR-002. **Parent** T064. **Witness** `runner_test`, fake Bazel
- [x] T006 Leg column in `required_tests.txt`; leg inventory and sandbox evidence; each required test in one Bazel leg. **Needs** T005. **Spec** FR-002, FR-023. **Parent** T066. **Witness** `verify_body_test` per leg
- [x] T007 CI receipt fields, except the base fields that T015 adds; reconcile the evidence-receipt contract. **Needs** T005. **Spec** FR-024, FR-015. **Parent** T065. **Witness** `runner_test`
- [x] T008 `ciparity`: legs from the registry equal jobs and `needs`; no inline policy; unique artifact names; read-only; no `pull_request_target`. **Needs** T006. **Spec** FR-003, FR-004, FR-024, FR-025. **Parent** T066. **Witness** `parity_test` mutations
- [x] T009 Workflow legs, per-attempt receipts; the rerun guide check against R4 stays with T023. **Needs** T008. **Spec** FR-004, FR-024. **Parent** T064, T065. **Witness** `parity_test`, `zizmor`, `actionlint`
- [x] T010 Gate fixtures by executing its run block: all success, skipped, cancelled, failed, absent, failed then rerun, partial rerun. **Needs** T009. **Spec** FR-005. **Parent** T061. **Witness** `parity_test`
- [x] T011 Merger in the quality module; `.bazelrc`, `bazelrc.allow`; no C++ or JDK. **Needs** T002, T003. **Spec** FR-009. **Parent** T063. **Witness** `bazelrc-policy`, analysis on this no-toolchain template
- [x] T012 `PlanBazel` verb rule; one invocation per stage; `coverage-floor` after it. **Needs** T011. **Spec** FR-006, FR-007. **Parent** T063. **Witness** Fake Bazel count; every required test under `coverage`
- [x] T013 Reports validated: Go statements from the raw profile; Node test entry and a `transport.ts` test; empty, placeholder, truncated, missing module, missing `.cover`, exit 3 with reports. **Needs** T012. **Spec** FR-019–FR-021. **Parent** T063. **Witness** Named fixtures, real tools
- [x] T014 Evaluator: profiles, measured source sets, enrollment, narrowing table R8, suppressions; `node:coverage` added to S005; real Git delete and add control. **Needs** T013. **Spec** FR-016–FR-018. **Parent** T060, T063. **Witness** Unit, real Git fixtures
- [x] T015 `from_base.sh`, `--base`: one resolution, lock, private output base, handle, every refusal in R6; same-tree positive. **Needs** T014. **Spec** FR-011–FR-015. **Parent** T062. **Witness** Real Git fixtures
- [x] T016 Declared `bff-template.policy-base`, installer, `stage.sh`; first push of a topic branch. **Needs** T015. **Spec** FR-012, FR-015a. **Parent** . **Witness** `fixtures_test`
- [ ] T017 **Anchor commit**: `floors-from-base` and `coverage-floor` registered, first floors from reconciled measurement; record the exit 11 control and targeted evidence. **Hold for independent targeted review**. **Needs** T016. **Spec** FR-011, FR-016. **Parent** T062, T063. **Witness** Review
- [x] T018 Base through `fresh_clone.sh` and CI; bounded measurement summary in the receipt; `escalate` and digest cases. **Needs** T017. **Spec** FR-020, FR-022, FR-023. **Parent** T062. **Witness** `runner_test`, `policy_base_test`, `parity_test`
- [x] T019 README, parent SC-001, contracts, hook limits. **Needs** T018. **Spec** FR-022. **Parent** SC-001. **Witness** Doc assertions
- [x] T020 S005 re-witness against the pinned tools; closes or restates parent T047. **Needs** T018. **Spec** FR-027. **Parent** T047. **Witness** Suppression fixtures
- [x] T021 Bypassed-hook commit fails a CI-equivalent run. **Needs** T018. **Spec** FR-027. **Parent** T070. **Witness** `fixtures_test`
- [x] T022 Trigger sentence narrowed. **Needs** T009. **Spec** FR-026. **Parent** . **Witness** Doc assertion
- [x] T023 `docs/quality/evidence.md`: separate claims; gaps for T067, T068, FR-023, SC-007. **Needs** T019–T022. **Spec** US5. **Parent** T069. **Witness** Review
- [x] T024 Reconcile capability states, classification, parent ledger; cheap checks. **Needs** T023. **Spec** all. **Parent** T070. **Witness** Review
- [x] T025 Reproduce R6 flows, with negatives, through the `policy` leg by reviewed reduction; a real compiled-base positive, and the combined negative: lowered floor plus replaced evaluator and patterns. **Needs** T024. **Spec** SC-004. **Parent** . **Witness** Logs
- [x] T026 Freeze; normal and hostile gates with `--base`; measure once; hold for independent acceptance. **Needs** T025. **Spec** SC-002. **Parent** T070. **Witness** Receipts
- [x] T027 Repair the measurement harness: a gofmt-clean edit, each run's log and receipt kept outside the clone, a failed passing scenario refused, offline recorded apart, private servers stopped before the work tree goes. **Needs** T026. **Spec** SC-002. **Parent** T070. **Witness** Stub and real-server controls, one `--edited-only` run

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
- T018: `33e6a55`, on the accepted anchor `e3b3b34`. Review of the anchor found that a passed `coverage-floor` left no measurements in the receipt; red first.
- T019 to T024: the commit after `33e6a55`. T020 closes parent T047: the pinned Node witness is in the [evidence record](../../docs/evidence/m6.md).
- Review of `33e6a55`: every leg's receipt binds the event's base, and a supplied base that cannot resolve denies PASSED in a leg that reads none. Red first.
- T025, T026: their boxes stayed open in the frozen source, because a file cannot record results about the commit that contains it. Results by scope are in the [evidence record](../../docs/evidence/m6.md). T025 replayed the flows through the `policy` leg, not as literal unsharded commands.
- T027: `a5ebd95`, then `e16718d` for the cleanup found in review.
