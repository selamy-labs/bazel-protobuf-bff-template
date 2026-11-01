# Tasks: Quality Registry, Runner and Lifecycle Hooks (M5)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[acceptance-matrix.md](acceptance-matrix.md)

This ledger, the plan and the matrix are committed before any production adapter code. Scratch
research came first and is recorded in the research file. Each commit names its tasks. A box is
ticked only when the named witness passes in the verifier; a ticked box is a record, not the evidence.

| Task | Needs | Work | Spec | Parent | Witness |
| --- | --- | --- | --- | --- | --- |
| [ ] T001 | — | Spec, research, plan, tasks, matrix, framework deltas | all | T090 | Review |
| [ ] T002 | T001 | Pin prek through the multitool lock | FR-015 | T003, T056 | `toolchain-fixtures`, T013 |
| [ ] T003 | T001 | nogo vet and staticcheck through x/tools 0.52.0 | FR-016 | T091 | `analysis-fixture` in `toolchain-fixtures` (root and quality module) |
| [ ] T004 | T001 | Pin ESLint and Prettier; `lint-ts`, `format-ts`; re-witness S002 spellings against pinned ESLint | FR-017 | T047, T054 | `//web:eslint_test`, `//quality/checks/suppression:eslint_witness_test` |
| [ ] T005 | T001 | Go binaries are static under the cgo-off configuration | FR-020 | FR-025 | `//bff/cmd:binaries_test`, with a dynamic control |
| [ ] T006 | T001 | Registry schema, instance, validation | FR-001, FR-002, FR-018 | T045, T050 | `@bff_template_quality//runner:runner_test` |
| [ ] T007 | T006 | Results, exit codes, diagnostics, receipts | FR-006, FR-007, FR-008 | T044, T053 | `runner_test` |
| [ ] T008 | T007 | Native pool, supervisor, Bazel invocation and translation, deadlines, slots, custody-gated server stop | FR-004, FR-005, FR-013, FR-014 | T044, T052 | `runner_test` with real processes and a fake Bazel |
| [ ] T009 | T007 | Inventory in Go; selectors; staged snapshot | FR-009, FR-009a, FR-012 | T051 | `runner_test`, cases carried from `inventory_test.sh` |
| [ ] T010 | T009, T008 | Pushed-commit worktree, escalation | FR-010, FR-011 | T051, T058 | `runner_test` with real Git |
| [ ] T011 | T008–T010 | Switch `verify.sh` to the runner; delete the old body, inventory and tool paths | FR-001, FR-003 | T054, T055 | `tools/verify.sh`, `fresh_clone.sh` normal and hostile |
| [ ] T012 | T011, T002 | `prek.toml`, `tools/hooks/stage.sh`, `//tools/hooks:install` | FR-015, FR-021 | T056, T057 | T013 |
| [ ] T013 | T012 | Hook fixtures: every row of the hook contract, real Git and prek, runner without Bazel; bootstrap ERROR scoped apart from verdicts | FR-009–FR-012, FR-015, FR-021 | T043 | `//tools/hooks:fixtures_test` |
| [ ] T014 | T013 | One real-Bazel commit, failing push, corrected push; deadline beside an independent producer; owned server stop | FR-010, FR-004 | T043 | `hook-e2e` in `full` |
| [ ] T015 | T011 | Named defect and corrected case per `GUARANTEE` | FR-018 | T046 | `runner_test` over the registry and fixture labels |
| [ ] T016 | T011, T012 | Classification, hooks and bypass documents | FR-021, FR-022 | T048, T049 | Review; `doc` assertions in fixtures |
| [ ] T017 | T014 | Measure stages cold, warm and edited, with failed attempts | FR-019 | T059, SC-006 | `docs/budgets/` record |
| [ ] T018 | T017 | Capability states, parent ledger, contracts, evidence, independent-review handoff | all | T090 | Review |

## Execution record

Filled in as work lands.
