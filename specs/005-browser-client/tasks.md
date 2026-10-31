# Tasks: Browser Client and End-to-End Tests (M4)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[acceptance-matrix.md](acceptance-matrix.md)

This ledger, the plan and the matrix were committed before any `web/` code. Each commit names its
tasks. Boxes are ticked only when the named witness passes in the verifier.

| Task | Needs | Work | Spec | Parent | Witness |
| --- | --- | --- | --- | --- | --- |
| [ ] T001 | — | Pin Chromium and its library closure | FR-006 | T030, FR-025 | `//web/e2e:browser_smoke_test` |
| [ ] T002 | — | Pin React, Vite, connect-web, Playwright | FR-006 | T039 | `lock_fixture.sh` |
| [ ] T003 | T002 | `web/client`: transport, credentials, error mapping, tests | FR-001, FR-003, FR-004 | T031, T039, FR-001 | `//web/client:errors_test` |
| [ ] T004 | T003 | `web/app`: form, lookup, builds | FR-001, FR-008 | T039, FR-001 | `//web/e2e:notes_e2e_test`, `//web/app:bundle_test` |
| [ ] T005 | T004 | `web/serve` static server | FR-005 | T039 | `//web/e2e:notes_e2e_test` |
| [ ] T006 | T001, T005 | Specs for every scenario | FR-002, FR-004, FR-005 | T032, FR-008 | `//web/e2e:notes_e2e_test` |
| [ ] T007 | T006 | Broken-mapping witness | FR-007 | T041, SC-002 | `//web/e2e:defect_e2e_test` |
| [ ] T008 | T005 | `//:dev` runner | FR-009 | T040 | `//tools/dev:dev_test` |
| [ ] T009 | T006–T008 | Inventories, docs, provenance, parent ledger | FR-010 | T025, T042 | `test-inventory`, `fresh_clone.sh` |
