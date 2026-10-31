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

| Task | Needs | Work | Spec | Parent | Witness |
| --- | --- | --- | --- | --- | --- |
| [x] T001 | — | Pin Node, pnpm, rules_js, rules_ts, gazelle; add JS and Go lockfiles | FR-012 | T005, FR-025 | `lock_fixture.sh`, `toolchain-fixtures` |
| [x] T002 | T001 | Spike: connect-go v2 through the generation rule | FR-003, FR-004 | T020, FR-010 | `//api:notes_connect_go` builds; research R3 |
| [x] T003 | — | Write the schema | FR-001, FR-002 | T021, FR-002 | `//api:validate_test` |
| [x] T004 | T001, T003 | `buf_generate`; Go and TypeScript targets | FR-003, FR-004 | T022, T023, FR-010 | `//api:compile_gate_test` |
| [x] T005 | T003 | Pin buf; canonical `buf.yaml`; `proto-format`, `proto-lint` | FR-005 | T018, FR-009 | `//api:schema_probes_test` |
| [x] T006 | T005 | Baseline reader and `proto-breaking` | FR-006, FR-007 | T017, T026, FR-009 | `@bff_template_quality//proto:proto_test` |
| [x] T007 | T006 | Breaking probes on the real schema | FR-008 | T016, FR-009 | `//api:schema_probes_test` |
| [x] T008 | T004 | Compile gate | FR-009 | T024, FR-010 | `//api:compile_gate_test` |
| [x] T009 | T003 | Boundary tests | FR-002 | T021, FR-003 | `//api:validate_test` |
| [x] T010 | T004 | Ban generated files and stray buf config; full clone in CI | FR-003, FR-005 | FR-010 | `//:bzlmod_policy_test`, `//:config_policy_test`, `//:ci_parity_test` |
| [x] T011 | T003 | API-style ledger | FR-010 | T019, FR-012 | human review of `docs/api-style.md` |
| [x] T012 | T005–T009 | Verifier inventories, provenance | FR-011 | FR-021 | `test-inventory`, receipt `expected_checks` |
| [x] T013 | T012 | Replace the bootstrap marker | FR-007 | T026 | `proto-breaking` compares with `bb57298` |
| [x] T101 | T006 | Witness and document every fresh-history route | FR-006, FR-007 | T072, FR-027 | `TestFreshHistoryCopy`, `api/README.md` |
| [x] T102 | T007 | Match rules as JSON types; show that a compile failure names no rule | FR-008 | T016 | `//api:schema_probes_test` |
| [x] T103 | — | Record this execution order honestly | — | constitution VI | this file, reviewed |

T025 in the parent stays open until M4 gives each RPC a production browser caller.
