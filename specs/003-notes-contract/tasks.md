# Tasks: Neutral Notes Contract and Build-Time Generation (M2)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md)

Parent task ids are in brackets.

- [x] T001 [T005] Pin Node, pnpm, rules_js, rules_ts and gazelle; add `package.json`, `pnpm-lock.yaml`, `pnpm-workspace.yaml`, `go.mod` and `go.sum`
- [x] T002 [T020] Spike: connect-go v2 builds through the generation rule (research R3)
- [x] T003 [T021] Write `api/template/notes/v1/notes.proto`
- [x] T004 [T022, T023] `buf_generate` rule; Go and TypeScript targets in `api/BUILD.bazel`
- [x] T005 [T018] Pin buf; canonical `quality/proto/buf.yaml`; `proto-format` and `proto-lint` in `tools/checks/native.sh`
- [x] T006 [T017, T026] Baseline reader tests and implementation in `quality/proto/`; `proto-breaking`
- [x] T007 [T016] Probes in `api/testdata/breaking/probes.tsv` and `tools/proto/probes_test.sh`
- [x] T008 [T024] Compile gate in `api/testdata/negative/`
- [x] T009 Boundary tests in `api/validate_test.go`
- [x] T010 Ban committed generated code and stray buf configuration; require a full clone in CI
- [x] T011 [T019] API-style ledger in `docs/api-style.md`
- [x] T012 Add the checks and tests to the verifier's inventories; update provenance
- [x] T013 Replace the bootstrap marker with the commit that introduces the schema
