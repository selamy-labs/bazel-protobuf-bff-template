# Quality baseline

The controls of the 2026-11-01 baseline against this repository (spec 011). A state is `enforced` only when the
named gate fails on a planted defect at the named revision and the restored control passes; `partial` when
something runs without gating or covering; `gap` when nothing runs. Closure needs every applicable row enforced;
a gap is an owned open task, never a waiver. Numbers are measured at the named revision, and no threshold is
written before its measurement. This table is reconciled with the registry and the evidence records in the
same change that alters any of them.

| Control | State | Gate | Measured, at | Threshold, source | Exclusions | Owner, task |
| --- | --- | --- | --- | --- | --- | --- |
| Pinned prek, formatting, lint | enforced | the hooks and the same checks at `full` | rejections at M5, M7, M8 | n/a | none | done |
| Complexity, Go | enforced | `complexity`, witness `complexity_test` | cognitive max 40, cyclomatic max 34 over 46 files and 233 declared functions at `bc97dcb`, and 253 functions once package-level literals are measured, maxima unchanged; the gate's first rejection was its own landing commit; the landing revision's full failed on the hook fixtures and is repaired in the next revision | `quality/complexity_budgets.json`: ceilings 20 and 19, the measured maxima outside five named debt functions, which are budgeted at their measured values; neither tool documents a default. A ceiling only falls | `_test.go`, `.pb.go`, `/testdata/`, each with a reason | T014 for the debt; T012a for the base comparator |
| Complexity, TypeScript | gap | none | not measured | none | none | T006 |
| Duplicate deletion, Go | partial | none | 0 pairs at 100 AST nodes at `00a4c6b`, planted pair detected (independent `dupl` 1.1.0) | none yet | none | T004, T011 |
| Duplicate deletion, TypeScript and Python | gap | none | not measured | none | none | T004 |
| Dead code | partial | ESLint unused rules for TypeScript only | not measured for Go | none | none | T005 |
| Strong typing | partial | Go vet; TypeScript `strict` and `noUncheckedIndexedAccess`; the suppression ban and the lint rules refuse directives and casts | every escape hatch inventoried with its reason in [type-escapes](type-escapes.md) at `e82e3bb`: none in TypeScript; in Go only checked assertions on JSON, syntax-tree, protobuf-clone and context values, and the generic JSON shape the CI-parity validator must read | n/a | none | `exactOptionalPropertyTypes` probe and the Python scripts' typing: T022 |
| Coverage, lines | partial | `coverage-floor`, `floors-from-base` | Go statements 87.9 root, 78.4 quality, 73.0 block-span entrypoints; TypeScript 100 compiled lines, 36 of 37 branches, at `446e5c8` | `quality/coverage_floors.json`, only rising | stated in the contract | T007 |
| Coverage, branch outcomes | gap, blocking | none | Go not produced by the toolchain; gobco 1.3.4 probed: a branch sub-metric that does not measure `select` | none | none | T007 |
| Property tests | partial | existing Go tests | Go only | n/a | none | T009 |
| Mutation testing | partial | none; named mutants by hand at M5 and M6 | not measured | none | none | T008 |
| Fakes with contract tests | partial | the fake's own tests | no contract suite | n/a | none | T010 |
| Config-driven behavior | gap | none | defaults not inventoried | none | none | T019 to T021 |
| Focused tests | partial | review | not measured | n/a | none | review |
| Accessibility | partial | `notes_e2e_test`: keyboard-only completion, the focus ring's computed outline, focus recovery to the first invalid field, accessible descriptions, the accessibility tree by role and name at `bed02b7` | structure, names, keyboard and focus only; no contrast, no screen-reader run, no full audit claimed | n/a | none | T026 done; a full audit is an open adoption decision |
| Feature, failure, recovery, integration acceptance | enforced | the end-to-end, failure, hook and image witnesses | per milestone | n/a | none | done |
| Independent exact-revision verification | enforced | review at each milestone's SHA | evidence records; M8 at `446e5c8` | n/a | none | done |
| Archive before cleanup | enforced for the future | `//:archive_ref_test` (27 controls), `//:archive_handoff_test` (9 controls) | prevention only: a named ref bundled, verified and restored to the exact commit and tree; a bare object id, a link, an existing or late-arriving destination and a damaged bundle refused; eligibility recorded only behind a verified archive. Nothing recovers `8ad7470` | n/a | none | done; cleanup custody stays with the lifecycle owner |
