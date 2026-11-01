# Tasks: Quality Registry, Runner and Lifecycle Hooks (M5)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[acceptance-matrix.md](acceptance-matrix.md)

This ledger, the plan and the matrix are committed before any production adapter code. Scratch
research came first and is recorded in the research file. Each commit names its tasks. A box is
ticked only when the named witness passes in the verifier; a ticked box is a record, not the evidence.

| Task | Needs | Work | Spec | Parent | Witness |
| --- | --- | --- | --- | --- | --- |
| [x] T001 | — | Spec, research, plan, tasks, matrix, framework deltas | all | T090 | Review |
| [x] T002 | T001 | Pin prek through the multitool lock | FR-015 | T003, T056 | `toolchain-fixtures`, T013 |
| [x] T003 | T001 | nogo vet and staticcheck through x/tools 0.52.0 | FR-016 | T091 | `analysis-fixture` in `toolchain-fixtures` (root and quality module) |
| [x] T004 | T001 | Pin ESLint and Prettier; `lint-ts`, `format-ts`; re-witness S002 spellings against pinned ESLint | FR-017 | T047, T054 | `//web:eslint_test`, `//quality/checks/suppression:eslint_witness_test` |
| [x] T005 | T001 | Go binaries are static under the cgo-off configuration | FR-020 | FR-025 | `//bff/cmd:binaries_test`, with a dynamic control |
| [x] T006 | T001 | Registry schema, instance, validation | FR-001, FR-002, FR-018 | T045, T050 | `@bff_template_quality//runner:runner_test` |
| [x] T007 | T006 | Results, exit codes, diagnostics, receipts | FR-006, FR-007, FR-008 | T044, T053 | `runner_test` |
| [x] T008 | T007 | Native pool, supervisor, Bazel invocation and translation, deadlines, slots, custody-gated server stop | FR-004, FR-005, FR-013, FR-014 | T044, T052 | `runner_test` with real processes and a fake Bazel |
| [x] T009 | T007 | Inventory in Go; selectors; staged snapshot | FR-009, FR-009a, FR-012 | T051 | `runner_test`, cases carried from `inventory_test.sh` |
| [x] T010 | T009, T008 | Pushed-commit worktree, escalation | FR-010, FR-011 | T051, T058 | `runner_test` with real Git |
| [x] T011 | T008–T010 | Switch `verify.sh` to the runner; delete the old body, inventory and tool paths | FR-001, FR-003 | T054, T055 | `tools/verify.sh`, `fresh_clone.sh` normal and hostile |
| [x] T012 | T011, T002 | `prek.toml`, `tools/hooks/stage.sh`, `//tools/hooks:install` | FR-015, FR-021 | T056, T057 | T013 |
| [x] T013 | T012 | Hook fixtures: every row of the hook contract, real Git and prek, runner without Bazel; bootstrap ERROR scoped apart from verdicts | FR-009–FR-012, FR-015, FR-021 | T043 | `//tools/hooks:fixtures_test` |
| [x] T014 | T013 | One real-Bazel commit, failing push, corrected push; deadline beside an independent producer; owned server stop | FR-010, FR-004 | T043 | `hook-e2e` in `full` |
| [x] T015 | T011 | Named defect and corrected case per `GUARANTEE` | FR-018 | T046 | `runner_test` over the registry and fixture labels |
| [x] T016 | T011, T012 | Classification, hooks and bypass documents | FR-021, FR-022 | T048, T049 | Review; `doc` assertions in fixtures |
| [x] T017 | T014 | Measure stages cold, warm and edited, with failed attempts | FR-019 | T059, SC-006 | `docs/budgets/` record |
| [x] T018 | T017 | Capability states, parent ledger, contracts, evidence, independent-review handoff | all | T090 | Review |
| [x] T019 | T011 | Stale-receipt removal at startup takes the runner's receipt lock; `flock` declared | FR-006, FR-007 | T053 | `//:verify_body_test`, `//:preflight_test` |

## Execution record

Tasks ran in this order, and each commit names its tasks. A tick records a witness that passed locally,
labeled `weakened`; the [evidence record](../../docs/evidence/m5.md) holds the results, and independent
review is pending.

- T001: plan `bbe88bb`, before production adapter code. Scratch probes are recorded in research.
- T002 to T005: prek pin `27bdd03`, analyzers `f7bb80f`, TypeScript lint and format `e2f4a13`, static
  binary witness `5759c44`.
- T006 to T010: runner `3a9a6f0`. The code came before its executable unit tests, so these tasks were not
  test-first. Review counterexamples were reproduced red, then repaired: fail-closed cases `0c62c21`,
  retrievable diagnostics `8609c56`.
- T011: adoption and adopted-orphan reaping `28e5329`. `verify_body.sh` stays as a thin wrapper, which
  changes the task's "delete the old body": research R10 says why.
- T012, T013: hooks and fixtures `47c3d90`, with `pass_filenames` and pushed-commit corrections after
  mutants survived.
- T014: `hook-e2e` `9e4a8ff`, with process scope `165d9db` after review.
- T015: `//:registry_witness_test`, with fixtures generated at test time, not in the directories the
  parent T046 listed: research R11 says why.
- T016: documents, with assertions in the fixtures.
- T017: stage record at `7366549`. Two earlier records were discarded; the evidence record says why.
- T018: this ledger, the contracts and the evidence. A review counterexample on spent contexts landed as
  `a2909a0`, red through its unit test first.

- After review of `405f93a`: the `escalate` list became the one definition of policy inputs, for
  escalation and for the digest. Red first over the real registry and real Git.
- After independent review of `41f2f98` (T019): the wrapper's startup removal took the runner's receipt
  lock. Red first, with the real runner as the owner.

Carried forward: the S005 coverage-marker re-witness in parent T047 moves to M6, so T047 stays open.
