# Tasks: Hermetic Skeleton and Bootstrap (M1)

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md)
**Parent mapping**: parent T001–T004 and T006–T015. Parent T005 moved to M2 (spec A4).

## Phase 1: Setup

- [x] T001 Root module: `.bazelversion` (9.3.0), `MODULE.bazel` (rules_go 0.64.2 with the Go SDK
  1.27.2 sha256 map, rules_shell 0.8.0, rules_multitool 1.11.1, buildifier_prebuilt 10.1.0),
  `REPO.bazel`, `BUILD.bazel`, `.gitignore`, `.bazelignore`. Parent T001, T002, T004.
- [x] T002 `.bazelrc` with:
  - `common --lockfile_mode=error`
  - platform configs; linux uses `--spawn_strategy=linux-sandbox`
  - `--sandbox_default_allow_network=false`
  - `build:weak-isolation`
  - no remote settings

  Research R2.
- [x] T003 [P] `multitool.lock.json`: gitleaks, typos, shellcheck, actionlint, zizmor for
  linux-amd64, linux-arm64 and darwin-arm64, with sha256 values from release metadata. Parent T003.
- [x] T004 Quality module `quality/MODULE.bazel` (`bff_template_quality` 0.1.0), consumed through
  `local_path_override`. Parent T011.

## Phase 2: Tests first (US2, US1)

- [x] T005 [P] [US2] Table tests for the suppression ban: one defect and one corrected fixture per
  code S001–S007, the path exemption, and the bounded output. File:
  `quality/checks/suppression/suppression_test.go`.
- [x] T006 [P] [US2] Native-check fixtures for gitleaks (a fake credential), typos (a misspelling)
  and the suppression-ban binary. Each has a defect case expected to fail and a corrected case
  expected to pass. Files: `tools/checks/native_fixtures_test.sh`, `tools/checks/testdata/`.
- [x] T007 [P] [US1] Bzlmod policy test: a WORKSPACE file is rejected, and an unlocked
  `MODULE.bazel` edit fails in a temp workspace that shares the repository cache. File:
  `tools/policy/bzlmod_test.sh`.
- [x] T008 [P] [US1] Spec Kit manifest test: it fails on a modified, extra or missing scaffold
  file. Files: `tools/speckit/scaffold_test.sh`, `tools/speckit/scaffold.sha256`.

## Phase 3: Implementation (US1, US2)

- [x] T009 [US2] Suppression-ban binary: `quality/checks/suppression/main.go` and `rules.go`. It
  reads a NUL-delimited path list on stdin (a file, not a TTY) and prints diagnostics of the form
  `suppression-ban path:line: [S00n] … (rerun: …)`.
- [x] T010 [US2] Tool configuration: `_typos.toml` and `.gitleaks.toml`, with the vendored-path
  exemption justified in comments.
- [x] T011 [US1] `bootstrap.sh manifest` subcommand. The `verify` subcommand also checks the
  manifest.
- [x] T012 [US1] `tools/verify.sh` and the `//:verify` target. Steps:
  1. preflight
  2. `bazel test //...`
  3. native checks over `git ls-files -co --exclude-standard -z`, each under `timeout` with
     `</dev/null`
  4. Spec Kit provenance, unless `--offline`
  5. receipt in `.verify/receipt.json`

  Flags: `--weak-isolation`, `--offline`. Parent T008.

## Phase 4: US3 Isolation preflight

- [x] T013 [US3] `tools/bootstrap/preflight.sh`. It checks:
  - Bazelisk/Bazel, Git and bash are present
  - free disk is at least 10 GB
  - the isolation probe passes, using the Bazel install base's `linux-sandbox` helper

  Each failure prints the remediations, marking which need admin rights. Parent T010.
- [x] T014 [US3] Isolation witness:
  - The default verify run fails on this host with the diagnostic.
  - `--weak-isolation` passes and the receipt is labeled `weakened`.

  Record both in `docs/evidence/m1-isolation.md`.

## Phase 5: US4 CI

- [x] T015 [US4] `.github/workflows/ci.yml`:
  - checkout pinned by SHA with `persist-credentials: false`
  - `permissions: contents: read`
  - a sysctl step that lifts the AppArmor restriction on the disposable runner
  - `bazel run //:verify` as the only policy step
  - a per-SHA concurrency group
  - an aggregate `gate` job with `if: always()`

  Parent T013.
- [x] T016 [US4] CI parity and gate fixture test: the only policy run step is `//:verify`, and
  every job is listed in `gate.needs`. File: `tools/ci/parity_test.sh`.
- [x] T017 [US4] Run actionlint, zizmor and shellcheck on the workflow and the scripts; these run
  inside verify's native checks.

## Phase 6: US1 fresh clone, US5 budgets

- [x] T018 [US1] `tools/bootstrap/fresh_clone.sh`. It clones `file://` HEAD into a temp directory
  with a fresh `--output_user_root` and repository cache, runs verify, and records the revision,
  outcome and isolation mode. Parent T014.
- [x] T019 [US5] `tools/bench/measure.sh` (R6). Parent T009.
- [x] T020 [US5] Measure and record `docs/budgets/linux-amd64-20c.json`: 3 cold runs, 5 warm runs,
  plus the network-free second run (SC-005). Parent T015.

## Phase 7: Polish

- [x] T021 Update the README quickstart for M1, and add `quality/README.md` and `tools/README.md`.
- [ ] T022 Run `bazel run //:verify -- --weak-isolation` at the final revision and commit the
  evidence record.

## Dependencies

- T001 and T002 come before everything else. T003 and T004 can run in parallel.
- T005–T008 are tests and come before T009–T012.
- T012 comes before T013–T014, T016–T018 and T020.
- T019 comes before T020.
