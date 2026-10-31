# Implementation Plan: Hermetic Skeleton and Bootstrap (M1)

**Branch**: `002-hermetic-skeleton` | **Date**: 2026-10-29 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-hermetic-skeleton/spec.md`

## Summary

M1 delivers a Bzlmod-only Bazel 9.3.0 root module and a separate `bff_template_quality` module
(0.1.0) that holds the first Go check (suppression ban). Native tools are pinned through
rules_multitool. A single entry point, `tools/verify.sh`, is exposed as `tools/verify.sh`, so
`tools/verify.sh` is the one command a user runs. It does the following in order:

1. Run the preflight.
2. Run `bazel test //...`, which covers the hermetic checks and the fixtures.
3. Run the native checks over the real file set.
4. Unless offline, check Spec Kit provenance.
5. Write a receipt that records the revision and the isolation mode.

Isolation fails closed by default, and reduced isolation is an explicit, labeled opt-in (research
R2). The milestone also includes a CI workflow with an aggregate gate, a fresh-clone witness and a
measurement harness.

## Technical Context

| Item | Choice |
| --- | --- |
| Language/version | Go 1.27.2 (SDK pinned by sha256); POSIX shell (bash) for orchestration; Starlark |
| Primary dependencies | Bazel 9.3.0; rules_go 0.64.2; rules_shell 0.8.0; rules_multitool 1.11.1; buildifier 10.1.0; gitleaks 8.30.1; typos 1.51.1; shellcheck 0.11.0; actionlint 1.7.12; zizmor 1.30.1; yq 4.54.1; uv 0.13.1 |
| Storage | N/A |
| Testing | `go_test` (table-driven) for the suppression ban; `sh_test` for the manifest, the Bzlmod policy and the native-check fixtures |
| Target platform | linux-amd64 (primary). linux-arm64 and darwin-arm64 are pinned but unverified. |
| Project type | Build/quality infrastructure (no application) |
| Performance goals | Measured, not asserted (R6) |
| Constraints | No host installs beyond Git, bash, curl and coreutils; sandbox network off; no remote cache |
| Scale | ~6 checks, 2 modules, 1 workflow |

## Constitution Check

| Principle | Status | Notes |
| --- | --- | --- |
| I. No vacuous surface | PASS | Node, pnpm, gazelle and prek are deferred until they have consumers. Go's consumer is the suppression ban. |
| II. Contracts first | N/A | No schemas in M1. |
| III. Hermetic, pinned | PASS | All verdict-affecting tools are pinned with sha256. coreutils, Git and bash are documented prerequisites. |
| IV. One policy, parity | PASS | CI runs only `tools/verify.sh`, and a parity test checks this. |
| V. Policy cannot weaken itself | PASS (scoped) | Suppression ban from day one. Base-built evaluation arrives in M6. |
| VI. Exact-revision evidence | PASS | The receipt records the revision and isolation mode. The CI aggregate requires every job. |
| VII. Guarantees vs signals | PASS | All M1 checks are guarantees. The manifest's limit is stated (R4). |
| VIII. Bounded, actionable | PASS | `timeout` wraps each native check; diagnostics name the check and give a rerun command; stdin comes from `/dev/null`. |
| IX. Public, neutral | PASS | No private content. |
| X. Smallest route | PASS | No rules_python for Spec Kit (R4). No remote cache. |

## Project Structure

```text
MODULE.bazel  MODULE.bazel.lock  .bazelversion  .bazelrc  BUILD.bazel  REPO.bazel  .gitignore
multitool.lock.json               # native tool pins (url + sha256 per platform)
_typos.toml  .gitleaks.toml       # tool configuration (versioned)
quality/                          # separate module bff_template_quality 0.1.0
├── MODULE.bazel  BUILD.bazel
└── checks/suppression/           # Go binary + table tests + fixtures
tools/
├── verify.sh                     # the one entry point (tools/verify.sh)
├── bootstrap/preflight.sh        # host capability preflight
├── bootstrap/fresh_clone.sh      # fresh-clone witness
├── policy/bzlmod_test.sh         # no WORKSPACE + lock-mismatch fixture
├── checks/native_fixtures_test.sh  # defect/corrected fixtures for gitleaks/typos/suppression
├── bench/measure.sh              # budgets
└── speckit/{bootstrap.sh,scaffold.sha256,scaffold_test.sh}
.github/workflows/ci.yml          # local only
docs/budgets/                     # measurement records
```

## Complexity Tracking

None.
