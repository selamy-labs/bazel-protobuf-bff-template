# Tasks: Neutral Notes Contract and Build-Time Generation (M2)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md),
[acceptance-matrix.md](acceptance-matrix.md)

## Execution record

This ledger does not describe the order the work happened in. M2 started with a toolchain spike and
it grew into the schema, generation and checks, committed in `bb57298`. The spec came first; this
plan, ledger and matrix were written after that code, and the boxes below were first ticked then.
T101 onward were planned first.

- Test-first was not followed for T005 to T009. Each test was written with its code or after it.
  Nothing here claims a red-then-green order, except where the matrix names an observed failure.
- `bb57298` and `4dfdc0e` carry no task-by-task mapping. The table below is that mapping, added later.

Credit therefore rests on each witness's own control, mutation and named reason, re-run at the
reviewed revision, not on the order of work.

## Order and traceability

Each task lists its dependencies, its spec 003 requirement, its parent task and requirement, and its
named witness.

- [x] T001 Pin Node, pnpm, rules_js, rules_ts, gazelle; add JS and Go lockfiles. **Spec** FR-012. **Parent** T005, FR-025. **Witness** `lock_fixture.sh`, `toolchain-fixtures`
- [x] T002 Spike: connect-go v2 through the generation rule. **Needs** T001. **Spec** FR-003, FR-004. **Parent** T020, FR-010. **Witness** `//api:notes_connect_go` builds; research R3
- [x] T003 Write the schema. **Spec** FR-001, FR-002. **Parent** T021, FR-002. **Witness** `//api:validate_test`
- [x] T004 `buf_generate`; Go and TypeScript targets. **Needs** T001, T003. **Spec** FR-003, FR-004. **Parent** T022, T023, FR-010. **Witness** `//api:compile_gate_test`
- [x] T005 Pin buf; canonical `buf.yaml`; `proto-format`, `proto-lint`. **Needs** T003. **Spec** FR-005. **Parent** T018, FR-009. **Witness** `//api:schema_probes_test`
- [x] T006 Baseline reader and `proto-breaking`. **Needs** T005. **Spec** FR-006, FR-007. **Parent** T017, T026, FR-009. **Witness** `@bff_template_quality//proto:proto_test`
- [x] T007 Breaking probes on the real schema. **Needs** T006. **Spec** FR-008. **Parent** T016, FR-009. **Witness** `//api:schema_probes_test`
- [x] T008 Compile gate. **Needs** T004. **Spec** FR-009. **Parent** T024, FR-010. **Witness** `//api:compile_gate_test`
- [x] T009 Boundary tests. **Needs** T003. **Spec** FR-002. **Parent** T021, FR-003. **Witness** `//api:validate_test`
- [x] T010 Ban generated files and stray buf config; full clone in CI. **Needs** T004. **Spec** FR-003, FR-005. **Parent** FR-010. **Witness** `//:bzlmod_policy_test`, `//:config_policy_test`, `//:ci_parity_test`
- [x] T011 API-style ledger. **Needs** T003. **Spec** FR-010. **Parent** T019, FR-012. **Witness** human review of `docs/api-style.md`
- [x] T012 Verifier inventories, provenance. **Needs** T005–T009. **Spec** FR-011. **Parent** FR-021. **Witness** `test-inventory`, receipt `expected_checks`
- [x] T013 Replace the bootstrap marker. **Needs** T012. **Spec** FR-007. **Parent** T026. **Witness** `proto-breaking` compares with `bb57298`
- [x] T101 Witness and document every fresh-history route. **Needs** T006. **Spec** FR-006, FR-007. **Parent** T072, FR-027. **Witness** `TestFreshHistoryCopy`, `api/README.md`
- [x] T102 Match rules as JSON types; show that a compile failure names no rule. **Needs** T007. **Spec** FR-008. **Parent** T016. **Witness** `//api:schema_probes_test`
- [x] T103 Record this execution order honestly. **Spec** —. **Parent** constitution VI. **Witness** this file, reviewed

T025 in the parent stays open until M4 gives each RPC a production browser caller.
