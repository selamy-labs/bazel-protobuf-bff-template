# Tasks: Go BFF for Notes (M3)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[acceptance-matrix.md](acceptance-matrix.md)

This ledger, the plan and the matrix were committed before any `bff/` code. Each commit names its
tasks. Boxes are ticked only when the named witness passes in the verifier.

| Task | Needs | Work | Spec | Parent | Witness |
| --- | --- | --- | --- | --- | --- |
| [x] T001 | — | Name the Go SDK repository; add `format-go`. `lint-go` deferred to M5 (research R7) | FR-014 | constitution IV | `//:native_fixtures_test` |
| [x] T002 | — | `ports.Store`, `memory` adapter and its tests | FR-004, FR-007 | T033, FR-005 | `//bff/adapters/memory:memory_test` |
| [x] T003 | T002 | `identity` package and tests | FR-008 | T036, FR-006 | `//bff/identity:identity_test` |
| [x] T004 | T002, T003 | `app` use cases and property tests | FR-001, FR-004 | T028, T034, FR-003 | `//bff/app:app_test` |
| [x] T005 | T004 | `testing/identity`, `testing/faults` | FR-009 | T036, FR-006 | `//bff/transport:transport_test` |
| [x] T006 | T004, T005 | Handlers, interceptors, CORS, mapping, with tests over a real server | FR-001, FR-003, FR-005–FR-008, FR-012 | T027, T035, FR-003–FR-005 | `//bff/transport:transport_test` |
| [x] T007 | T006 | `server`: startup refusal, ready event, shutdown | FR-009, FR-010, FR-013 | T029, T037, FR-006 | `//bff/server:server_test` |
| [x] T008 | T007 | Production and dev binaries | FR-009–FR-011, FR-013 | T029, T037, FR-006 | `//bff/cmd:binaries_test` |
| [x] T009 | T008 | Dependency rules | FR-002, FR-011 | T038, FR-007 | `//bff:dependency_rules_test` |
| [x] T010 | T006–T009 | Inventories, docs, provenance, parent ledger | FR-015 | FR-021 | `test-inventory`, receipt |
| [x] T011 | T006 | Review regression: downstream errors cannot label themselves public | FR-006 | T035, FR-004 | `TestDownstreamCannotLabelItsOwnErrorPublic`, `TestDownstreamSentinelsMapToConstantText`, `TestFrameworkErrorsStayPublic` |

## Execution record

`bff/` was written after this ledger, as planned. The tests were written alongside the code, not
before it, so the first passing run proves little by itself. Credit comes from two later witnesses:

- A mutation sweep broke 14 security behaviors one at a time. Each was caught by its intended test,
  and none was a compile failure.
- T011 came from independent review. The regression was written first and observed to fail, 48 of 48
  cases, before the fix.
