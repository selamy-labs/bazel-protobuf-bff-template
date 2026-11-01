# Tasks: Go BFF for Notes (M3)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[acceptance-matrix.md](acceptance-matrix.md)

This ledger, the plan and the matrix were committed before any `bff/` code. Each commit names its
tasks. Boxes are ticked only when the named witness passes in the verifier.

- [x] T001 Name the Go SDK repository; add `format-go`. `lint-go` deferred to M5 (research R7). **Spec** FR-014. **Parent** constitution IV. **Witness** `//:native_fixtures_test`
- [x] T002 `ports.Store`, `memory` adapter and its tests. **Spec** FR-004, FR-007. **Parent** T033, FR-005. **Witness** `//bff/adapters/memory:memory_test`
- [x] T003 `identity` package and tests. **Needs** T002. **Spec** FR-008. **Parent** T036, FR-006. **Witness** `//bff/identity:identity_test`
- [x] T004 `app` use cases and property tests. **Needs** T002, T003. **Spec** FR-001, FR-004. **Parent** T028, T034, FR-003. **Witness** `//bff/app:app_test`
- [x] T005 `testing/identity`, `testing/faults`. **Needs** T004. **Spec** FR-009. **Parent** T036, FR-006. **Witness** `//bff/transport:transport_test`
- [x] T006 Handlers, interceptors, CORS, mapping, with tests over a real server. **Needs** T004, T005. **Spec** FR-001, FR-003, FR-005–FR-008, FR-012. **Parent** T027, T035, FR-003–FR-005. **Witness** `//bff/transport:transport_test`
- [x] T007 `server`: startup refusal, ready event, shutdown. **Needs** T006. **Spec** FR-009, FR-010, FR-013. **Parent** T029, T037, FR-006. **Witness** `//bff/server:server_test`
- [x] T008 Production and dev binaries. **Needs** T007. **Spec** FR-009–FR-011, FR-013. **Parent** T029, T037, FR-006. **Witness** `//bff/cmd:binaries_test`
- [x] T009 Dependency rules. **Needs** T008. **Spec** FR-002, FR-011. **Parent** T038, FR-007. **Witness** `//bff:dependency_rules_test`
- [x] T010 Inventories, docs, provenance, parent ledger. **Needs** T006–T009. **Spec** FR-015. **Parent** FR-021. **Witness** `test-inventory`, receipt
- [x] T011 Review regression: downstream errors cannot label themselves public. **Needs** T006. **Spec** FR-006. **Parent** T035, FR-004. **Witness** `TestDownstreamCannotLabelItsOwnErrorPublic`, `TestDownstreamSentinelsMapToConstantText`, `TestFrameworkErrorsStayPublic`

## Execution record

`bff/` was written after this ledger, as planned. The tests were written alongside the code, not
before it, so the first passing run proves little by itself. Credit comes from two later witnesses:

- A mutation sweep broke 14 security behaviors one at a time. Each was caught by its intended test,
  and none was a compile failure.
- T011 came from independent review. The regression was written first and observed to fail, 48 of 48
  cases, before the fix.
