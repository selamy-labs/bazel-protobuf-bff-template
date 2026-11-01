# Tasks: Generic Bazel/Protobuf BFF Template — Foundation

**Input**: Design documents from `/specs/001-bff-template-foundation/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Required. The constitution requires test witnesses written before or with behavior,
each with a defect case and its corrected counterpart. Test tasks come first in each story.

**Organization**: Phases follow milestones (spec Milestone Map). US2 (contracts) is scheduled
before US1 because the browser→BFF slice consumes the generated contract. Both are P1.
**Status**: task boxes record progress. M2 revised T018 and T022: generation uses the pinned buf
through `tools/proto/defs.bzl`, and the schema lives at `api/template/notes/v1/`
(spec 003 research R1, R4).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (US1–US7); setup/foundational/polish have none
- Each task names its files; **Witness** names the acceptance evidence (see acceptance-matrix.md)

## Phase 1: Setup — M1 hermetic skeleton

**Purpose**: Bootstrap from Git, bash, curl and coreutils with every verdict-affecting tool pinned.

- [x] T001 Create `MODULE.bazel`, `.bazelversion` (9.x), `.bazelrc` (`--lockfile_mode=error`, strict action env, sandbox network off, disk cache path neutral) and root `BUILD.bazel`; commit `MODULE.bazel.lock`
- [x] T002 [P] Add `REPO.bazel`, `.gitignore`, `.bazelignore` and `.editorconfig`
- [x] T003 [P] Pin native tools with digests in `multitool.lock.json` (gitleaks, typos, shellcheck, actionlint, zizmor, prek, buildifier) via rules_multitool
- [x] T004 Register Go SDK via rules_go and gazelle in `MODULE.bazel`; add `go.mod`/`go.sum`
- [x] T005 Register Node, pnpm, aspect_rules_js/rules_ts in `MODULE.bazel`; add `package.json`, `pnpm-lock.yaml`, `pnpm-workspace.yaml` — **moved to M2 (with T023)**: it needs a real consumer (spec 002 A4)
- [x] T006 [P] Add lockfile/Bzlmod policy test rejecting WORKSPACE files and unlocked changes in `tools/policy/bzlmod_test.sh`
- [x] T007 [P] Port `tools/speckit/bootstrap.sh verify` to an offline Bazel test with a hash-pinned Spec Kit archive in `tools/speckit/BUILD.bazel` and `tools/speckit/upstream.json` — **revised in spec 002 R4**: an offline hash-manifest test plus a networked provenance check required in CI; a fully offline regeneration would need rules_python
- [x] T008 Add `tools/verify.sh` (initially T006, T007 and T012 checks) in `BUILD.bazel`
- [x] T009 [P] Add measurement harness `tools/bench/measure.sh` (cold/warm, N runs, p50/p95, peak RSS, host profile JSON)
- [x] T010 [P] Add host capability preflight (user namespaces, disk, prerequisites) with actionable messages in `tools/bootstrap/preflight.sh`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Minimal honest gates and CI before any story code. ⚠️ Blocks all stories.

- [x] T011 Create the separate shared module `quality/MODULE.bazel` (`bff_template_quality`, version 0.1.0) and consume it with `local_path_override` in root `MODULE.bazel`
- [x] T012 [P] Minimal guarantees `secrets` (gitleaks), `suppression-ban` and `typos` as Bazel test targets in `quality/checks/` with defect + corrected fixtures (replaced by the registry in T055)
- [x] T013 Local CI workflow `.github/workflows/ci.yml`: checkout exact head SHA with `persist-credentials: false`, `tools/verify.sh` as the only policy step (no extra inline commands), aggregate `gate` job (`if: always()`); actions SHA-pinned; per-SHA concurrency. Not pushed.
- [x] T014 Fresh-clone witness `tools/bootstrap/fresh_clone_test.sh`: clone into temp dir with empty Bazel output base/caches and no credentials, run `tools/verify.sh`, record revision and outcome. **Witness** SC-001
- [x] T015 Record first cold/warm measurements for bootstrap and `tools/verify.sh` in `docs/budgets/linux-x86_64.json`. **Witness** FR-026

**Checkpoint**: fresh clone verifies with zero credentials; CI file lints with actionlint/zizmor.

---

## Phase 3: User Story 2 — Contract changes are safe (Priority: P1) — M2

**Goal**: neutral schema with compatibility, lint, format and build-time generation for Go and TS.

**Independent Test**: breaking probes fail, compatible addition passes, both consumers compile.

### Tests for User Story 2 ⚠️ write first, observe failing

- [x] T016 [P] [US2] Breaking-probe fixtures (field number, type, rename, json_name, removed field without reserve) and compatible-addition fixture in `api/testdata/breaking/`
- [x] T017 [P] [US2] Baseline-reader tests: immutable SHA via git objects, missing baseline fails closed (exit 4), shallow clone message, explicit bootstrap marker in `quality/proto/baseline_test.go`

### Implementation for User Story 2

- [x] T018 [US2] buf configuration (`api/buf.yaml`, STANDARD lint, FILE breaking) via rules_buf targets in `api/BUILD.bazel`
- [x] T019 [P] [US2] API-style ledger with applicable AIP rules and justified exceptions in `docs/api-style.md`
- [x] T020 [US2] Spike: connect-go newest v2.x (≥14 days old) with rules_go/gazelle; fall back to v1.21.x only on documented failure; record outcome in `research.md` R-04
- [x] T021 [US2] Write `api/notes/v1/notes.proto` per `contracts/notes-api.md` with protovalidate rules
- [x] T022 [US2] Go codegen: `go_proto_library` with connect compiler in `api/notes/v1/BUILD.bazel`
- [x] T023 [US2] TS codegen with protoc-gen-es v2 rule in `api/notes/v1/BUILD.bazel` and `tools/proto/defs.bzl`
- [x] T024 [US2] Negative compile gate: TS caller using wrong request type must fail to compile, in `api/testdata/negative/`
- [x] T025 [US2] Contract-consumer evidence without building the #28 analyzer: each RPC has a production browser caller exercised by e2e (guarantee, via T032) plus a review checklist in `docs/api-style.md`; **adopt the py#28 profile when released** (still open upstream) and contribute this repo's Protobuf/Bazel case as its witness (consumer, fake, generated-only, unsupported path)
- [x] T026 [US2] Implement baseline reader to pass T017 in `quality/proto/baseline.go`; wire `proto-breaking` into `tools/verify.sh`

**Checkpoint**: SC-003 fixtures pass at exact revision; evidence recorded.

---

## Phase 4: User Story 1 — Fresh clone shows the browser→BFF slice (Priority: P1) 🎯 MVP — M3+M4

**Goal**: real React client calls the Go BFF through generated Connect clients; success and typed
failures are visible in the browser.

**Independent Test**: on a fresh clone, `bazel test //web/e2e:all` passes all four outcome
scenarios; the deliberately broken variant fails.

### Tests for User Story 1 ⚠️ write first, observe failing

- [x] T027 [P] [US1] BFF handler tests over in-process Connect server (success, invalid, not found, cross-principal not found, unauthenticated, deadline, unavailable) in `bff/transport/handlers_test.go`
- [x] T028 [P] [US1] Property tests for title/body validation and name parsing in `bff/app/validate_test.go`
- [x] T029 [P] [US1] Startup tests: test identity refused in production mode; production without verifier fails closed in `bff/cmd/bff/config_test.go`
- [x] T030 [US1] Spike: pinned Playwright browser under Bazel sandbox (rules_playwright vs pinned archive), offline after fetch; record in `research.md` R-05
- [x] T031 [P] [US1] Client unit tests: Connect error → UI outcome mapping in `web/client/errors.test.ts`
- [x] T032 [P] [US1] E2E specs for every scenario in `contracts/notes-api.md` (success, invalid title, body too long, not found, cross-principal not found, unauthenticated, cookie-only request rejected, unavailable, deadline exceeded) in `web/e2e/notes.spec.ts`

### Implementation for User Story 1

- [x] T033 [US1] Storage port and in-memory adapter in `bff/ports/store.go`, `bff/adapters/memory/store.go`
- [x] T034 [US1] Use cases, protovalidate enforcement and error mapping in `bff/app/notes.go`
- [x] T035 [US1] Connect handlers, exact-origin CORS, default/max deadlines in `bff/transport/`
- [x] T036 [US1] `Authenticator` boundary; `testonly` test identity in `bff/testing/identity.go`
- [x] T037 [US1] `bff/cmd/bff/main.go`: mode config, ready event on stdout, graceful shutdown, no stdin
- [x] T038 [US1] Dependency-rules test (directions, testonly fencing, no `web` import from `bff`) in `quality/checks/dependency_rules_test.sh`
- [x] T039 [US1] Client wrapper and transport in `web/client/`; React form + lookup in `web/app/`; Vite build/serve via rules_js
- [x] T040 [US1] `//:dev` runner starting BFF and web together in `tools/dev/`
- [x] T041 [US1] Deliberate-defect witness: mutate error mapping (e.g. not_found→internal) in a fixture build; e2e must fail naming the scenario; restore passes. In `web/e2e/defect_test.sh`
- [x] T042 [US1] Checkpoint: fresh-clone browser path (T014 extended) + evidence at exact revision. **Witness** SC-001, SC-002

---

## Phase 5: User Story 3 — Lifecycle hooks (Priority: P2) — M5 (may start after Phase 2)

**Goal**: one registry, one runner, prek stages, safe and bounded.

**Independent Test**: hook fixtures in temporary repositories (contracts/hook-stages.md) all pass.

### Tests for User Story 3 ⚠️ write first

- [x] T043 [P] [US3] Hook fixture harness in temp repos covering every fixture in `contracts/hook-stages.md` (incl. untracked-file, failing/corrected push, malformed config) in `quality/hooks/fixtures_test.sh`
- [x] T044 [P] [US3] Runner tests: exit codes and Bazel-code translation, diagnostic format, 50-line bound, deadline handling (Bazel client SIGINT/shutdown, native process group, `setsid` child reaped via subreaper), closed stdin, zero-input GUARANTEE at `full` → exit 12 in `quality/runner/runner_test.go`
- [x] T045 [P] [US3] Registry validation tests (duplicate id, missing `full`, SIGNAL with failing threshold, GUARANTEE without fixtures, missing classification) in `quality/registry/validate_test.go`
- [x] T046 [P] [US3] Defect + corrected fixture per GUARANTEE check in `quality/fixtures/<check-id>/`
- [x] T047 [P] [US3] Suppression pattern fixtures per language (Go, TS, proto, Starlark, shell) in `quality/fixtures/suppression-ban/`; re-witness every S002 and S005 spelling, including multi-line and quoted forms, with the pinned ESLint and coverage tool once they exist S005 re-witnessed against the pinned Node in spec 007 T020.

### Implementation for User Story 3

- [x] T048 [P] [US3] Guarantee-vs-signal table and human-review list in `docs/quality/classification.md`
- [x] T049 [P] [US3] Accurate bypass documentation (`--no-verify`, `SKIP`/`PREK_SKIP`) in `docs/quality/hooks.md`
- [x] T050 [US3] Registry schema `quality/proto/registry.proto` and instance `quality/registry.textproto`
- [x] T051 [US3] Runner stage selection: staged-blob input for pre-commit, pushed-commit worktree for pre-push (`PRE_COMMIT_TO_REF`), NUL-safe paths, in `quality/runner/select.go`
- [x] T052 [US3] One Bazel invocation per stage, bounded native pool, deadlines, subreaper in `quality/runner/exec.go`
- [x] T053 [US3] Diagnostics and JSON receipt writer in `quality/runner/report.go`
- [x] T054 [US3] Adapters mapping registry entries to pinned tools in `quality/adapters/{go,ts,proto,docs,shell}/`
- [x] T055 [US3] Move T012 checks into the registry and delete their standalone targets (adopt-and-delete)
- [x] T056 [US3] `prek.toml` with one hook per stage (`full` as prek `manual`), prek pinned via multitool; cite the py#25/ts#20 contract revision followed
- [x] T057 [US3] `//tools/hooks:install` thin wrapper over pinned `prek install` (legacy-hook handling verified by fixture, `core.hooksPath`, worktrees); no custom chaining
- [x] T058 [US3] Policy-change escalation to `full` at pre-push only in `quality/runner/select.go`
- [x] T091 [US3] Go analyzers (`lint-go`): `nogo` under rules_go 0.64.2 cannot read Go 1.27 export data (spec 004 research R7). Move to a rules_go release that can, once it meets the 14-day pin rule, and enable vet then staticcheck. **Witness**: a vet finding in a fixture fails the Go build, and its corrected counterpart passes
- [x] T059 [US3] Checkpoint: fixtures pass; measure stage budgets cold/warm (1, 50, 500 files) into `docs/budgets/`. **Witness** SC-005, SC-006

---

## Phase 6: User Story 4 — Exact-revision evidence and weakening detection (Priority: P2) — M6

**Goal**: CI parity, single aggregate, receipts, interim weakening detection; adopt #26/#27.

**Independent Test**: weakening and evidence fixtures produce documented verdicts.

### Tests for User Story 4 ⚠️ write first

- [x] T060 [P] [US4] Base-built evaluator fixtures: proposal lowers a coverage floor, adds a suppression, edits the suppression pattern set, or replaces the evaluator code → `floors-from-base` still fails; positive: floor raised, unrelated tool update passes; in `quality/fixtures/floors-from-base/`
- [x] T061 [P] [US4] CI shard/aggregate fixtures: one shard skipped, cancelled, or failed-then-rerun-succeeded (latest attempt counts), receipt missing → gate result as documented, in `quality/fixtures/aggregate/`

### Implementation for User Story 4

- [x] T062 [US4] CI `policy` shard: build `//quality/...` from a worktree at `base_revision`, run it against the proposal tree, read-only permissions, no secrets, in `.github/workflows/ci.yml` and `quality/policy/from_base.sh`
- [x] T063 [US4] Coverage floors that only rise vs base in `quality/checks/coverage_floor.go` and `quality/coverage_floors.json`
- [x] T064 [US4] Complete CI as registry shards only (`//quality:run --stage=full --shard=build-test|e2e|coverage|policy`), read-only permissions, aggregate `gate` on latest attempts; in `.github/workflows/ci.yml`
- [x] T065 [US4] Per-job receipts (contracts/evidence-receipt.md) uploaded with `if-no-files-found: error`
- [x] T066 [US4] Parity check: every CI policy step is a `--stage=full --shard=` call, union of shards == `full`, no inline policy commands, in `quality/checks/ci_parity_test.sh`
- [ ] T067 [US4] **Blocked on py#26 release**: adopt the shared policy-diff/trusted-baseline contract and its fixtures (removed check/stage, narrowed scope, tampering, stale/self-issued authorization, authorized exception, malformed evidence); delete any interim code it supersedes
- [ ] T068 [US4] **Blocked on py#27 release**: adopt the shared evidence validator and its fixtures (running producer, truncated report, omitted leg, old revision, contradictory, moved branch, review receipt vs self-assertion); closes FR-023/SC-007. Until then record the gap in `docs/quality/evidence.md`
- [x] T069 [US4] Document separate claims (offline / local / hosted CI / human approval / deployment) in `docs/quality/evidence.md`
- [x] T070 [US4] Checkpoint (M6 exit = aggregate + receipts + base-built floors): T060/T061 pass; bypassed-hook commit fails in CI-equivalent run. SC-007 is not an M6 exit criterion

---

## Phase 7: User Story 5 — Second consumer and upgrade (Priority: P3) — M7

- [x] T071 [US5] Release procedure for `bff_template_quality`: immutable tag commit, release notes with rule-change table generated from registry diff, in `quality/RELEASING.md` and `quality/tools/rule_diff.go`
- [x] T072 [US5] Second neutral consumer as a **separate Git repository** (different neutral domain; local until M8, consuming quality vN via `git_override` and prek `repo`/`rev` against a `file://` remote; re-proven against the hosted remote after publication)
- [x] T073 [US5] Upgrade the consumer vN → vN+1 by one pin change; record diff and verifier outcome. **Witness** SC-008
- [x] T074 [P] [US5] Language adapter profile with Python worked design in `docs/extending/language-profile.md` (no Python code without a consumer)
- [x] T075 [US5] Checkpoint: declare quality interface stable (1.0.0) only if T073 passes

---

## Phase 8: User Story 7 — Public-release readiness (Priority: P3) — M8

- [ ] T076 [US7] Neutrality scan: generic secret/identifier patterns in CI plus operator-supplied private denylist read from outside the repository (never committed) in `quality/checks/neutrality.sh`
- [ ] T077 [P] [US7] Provenance inventory check (every vendored/adapted path has an entry) against `docs/provenance.md`
- [ ] T078 [US7] Owner decision recorded: name, visibility, license (A5); add `LICENSE`
- [ ] T079 [US7] Declarative settings for the new repository only (incl. its initial ruleset — explicit carve-out; never touches existing repositories) with OpenTofu GitHub provider in `governance/github/` (fmt/validate/test offline; **no apply**)
- [ ] T080 [US7] Execute `docs/public-release-review.md` and record result. **Witness** SC-009. Publication itself is an operator gate outside this repo.

---

## Phase 8a: BFF container image — M8a

Tasks are in [spec 009](../009-bff-image/tasks.md). They land before M8, so that M8's inventory and scans cover the image.

---

## Phase 9: User Story 6 — Optional GCP profile (Priority: P3) — M9 (optional)

- [ ] T081 [US6] Bootstrap module (state bucket, local→remote state migration runbook) in `deploy/gcp/bootstrap/`
- [ ] T082 [US6] Runtime module (Artifact Registry, Cloud Run, least-privilege service accounts) in `deploy/gcp/runtime/`
- [ ] T083 [US6] Cloud Build pipelines: invoke the Bazel image target, push its digest; deploy trigger with `approvalRequired` in `deploy/gcp/cloudbuild/`
- [ ] T084 [US6] Offline `tofu fmt/validate/test` with mocked providers + tflint, opt-in registry stage `deploy-gcp`
- [ ] T085 [US6] Operator runbook listing every gate (apply, approval, IAM) in `deploy/gcp/README.md`; no values committed

---

## Phase 10: Optional worker-integration conformance — M10 (deferred)

- [ ] T086 Consume py#29 profile when released: document how the runner's deadlines, stdin closure and failure taxonomy satisfy it in `docs/extending/worker-conformance.md`; no runtime, no paid calls

---

## Phase 11: Polish & Cross-Cutting Concerns

- [ ] T087 [P] README per readme-excellence (purpose, quickstart that runs, owner) in `README.md`
- [ ] T088 [P] Directory READMEs for `api/`, `bff/`, `web/`, `quality/`
- [ ] T089 Re-measure all budgets; declare budgets = measured p95 + documented margin in `docs/budgets/`
- [ ] T090 Run `/speckit-analyze` for cross-artifact consistency before each milestone review
- [ ] T092 Optional host profile: queued until a reviewed producer revision and a complete descriptor exist, per `contracts/host-profile.md`. Qualify with the existing commands unchanged, and primary receipts. **Witness** FR-035

---

## Dependencies & Execution Order

```text
Phase 1 (T001–T010) → Phase 2 (T011–T015) ─┬─> Phase 3 US2 (T016–T026) → Phase 4 US1 (T027–T042) ─┐
                                            └─> Phase 5 US3 (T043–T059) ─────────────────────────────┴─> Phase 6 US4 (T060–T070)
Phase 6 → Phase 7 US5 (T071–T075) → Phase 8 US7 (T076–T080)
Phase 6 → Phase 9 US6 (optional)   Phase 6 → Phase 10 (optional, blocked on #29)
```

Within phases: T004/T005 before T008; T011 before T012; T020 before T022; T021 before T022–T025;
T030 before T032/T039; T033 → T034 → T035 → T037; T050 before T051–T058; T062 before T067.
Blocked externally: T025 adoption part (#28), T067 (#26), T068 (#27), T086 (#29), T092 (a reviewed host producer).

## Parallel Example: User Story 1

```text
T027 bff/transport/handlers_test.go   T028 bff/app/validate_test.go
T029 bff/cmd/bff/config_test.go       T031 web/client/errors.test.ts   T032 web/e2e/notes.spec.ts
```

## Implementation Strategy

1. **MVP** = Phases 1–4: hermetic bootstrap, contract, BFF, browser e2e with minimal gates (T012,
   T013). Stop and review with evidence at the exact revision.
2. Phase 5 can run in parallel with Phases 3–4 (different files: `quality/` vs `api/ bff/ web/`).
3. Phase 6 joins both lines; Phase 7 is the gate for declaring the shared interface stable.
4. Phases 8–10 are release/optional work and require owner decisions or upstream releases.

## Notes

- Commit after each task or logical group; each commit names its task id.
- A checkpoint is complete only with full-verifier evidence at the exact revision.
- Never implement upstream-owned contracts (#25/#20 Python/TS adapters, #26 policy-diff, #27 evidence validator, #28 analyzer, #29 worker profile) here.
