# Upstream Issue Map

All eight issues were read on 2026-10-28; all were OPEN with no comments. The RFC (#30) and the
tracker (#31) link the work. **Ownership rule**: an issue that defines a shared contract owns the
implementation upstream. This repository consumes that contract and does not build a parallel one.
Not every issue lands in the first slice.

| Issue | Ownership here | Requirements | Milestone | Acceptance witnesses (tasks) | Explicitly deferred / not done here |
| --- | --- | --- | --- | --- | --- |
| [py#30](https://github.com/selamy-labs/relentless-ci-python/issues/30) RFC: polyglot Bazel/Protobuf BFF template | **Primary**. This repository is the RFC's design response | FR-001–FR-012, FR-025–FR-030 | M0 design; M1–M4 first slice; M7 second consumer | T014, T026, T041, T042, T073 | Remote cache, multiworker, control plane (excluded by the RFC); provider deploy optional (M9) |
| [py#31](https://github.com/selamy-labs/relentless-ci-python/issues/31) Tracker | Traceability only | FR-020, FR-024, FR-032 | all | acceptance-matrix.md rows link back | Does not dispatch or change settings |
| [py#25](https://github.com/selamy-labs/relentless-ci-python/issues/25) Python prek adapter | **Consumer of the shared stage/diagnostic contract**; implements only the Bazel adapter | FR-015–FR-018 | M5 | T043, T044, T056–T059 | Python adapter implementation stays upstream; commit-msg stage off until adopted |
| [ts#20](https://github.com/selamy-labs/relentless-ci-typescript/issues/20) TypeScript prek adapter | Same as #25; TS checks here run through Bazel/pnpm-locked tools | FR-013–FR-018 | M5 | T043, T046, T054 | npm/Mise TS adapter stays upstream |
| [py#26](https://github.com/selamy-labs/relentless-ci-python/issues/26) Policy weakening vs trusted baseline | **Consumer**. Local only: suppression bans and floors-only-rise, judged by an evaluator built from the base revision | FR-019 | M5 (bans), M6 (base-built floors) | T047, T060, T062, T063; T067 adoption | Shared policy-diff contract and authorization binding stay upstream. **Not reinstated**: custom trusted-policy issuers (py PR #18, ts PR #16). No App, no branch-protection change, no review-count change |
| [py#27](https://github.com/selamy-labs/relentless-ci-python/issues/27) Exact-revision evidence | **Consumer**. Producers emit compatible receipts; single CI aggregate on latest attempts | FR-021, FR-022, FR-024 (M6); FR-023 on adoption | M6; T068 | T061, T064, T065, T069; T068 adoption closes SC-007 | Shared format, validator, attempt/readiness semantics and the evidence fixture set stay upstream; no scheduler, signing service or App |
| [py#28](https://github.com/selamy-labs/relentless-ci-python/issues/28) Contract consumers and Protobuf/Bazel profile | **Consumer**. The analyzer and review contract stay upstream; this repo uses native schema gates and contributes its real Protobuf/Bazel case as a witness for the upstream profile | FR-009–FR-012, FR-007 | M2–M4; T025 adoption | T016, T024, T025, T032, T038 | Analyzer implementation; AIP-151 operations (no async work in the example); REST mappings added only to silence linters; migrating the library templates to Bazel |
| [py#29](https://github.com/selamy-labs/relentless-ci-python/issues/29) Bounded execution and identity/billing | **Partial consumer**. The runner implements bounded execution (deadlines, closed stdin, process groups). The identity/billing profile is deferred | FR-017, FR-031 | M5 (bounds), M10 (profile) | T044, T086 | Worker profile, quota/billing taxonomy, notification adapter: deferred to M10 after the upstream release; never a runtime |

## Assessment of overlap risk

- **#25/#20 vs M5**: the overlap is in the stage names, exit codes and diagnostic line, and
  agreeing on those is intended. The risk is a forked definition. Mitigation: `contracts/hook-stages.md`
  is labeled as following the upstream contract, and T056 must cite the upstream revision it
  follows.
- **#26 and #27 vs M6**: locally, M6 ships only suppression bans, base-built floors, receipts and
  the aggregate gate. It builds no comparator, evidence validator or evidence fixture set. The
  M6 exit criteria do not depend on upstream, so M7 and M8 are not blocked. FR-023 and SC-007 close
  when #27 is adopted.
- **#28**: this repository builds no analyzer. It offers its Protobuf/Bazel case to the upstream
  profile as a witness, and that case keeps its provenance if it moves upstream.
