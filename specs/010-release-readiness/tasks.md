# Tasks: Public-release readiness and provisioning plan

**Input**: Design documents from `specs/010-release-readiness/`
**Prerequisites**: plan.md, spec.md, research.md, acceptance-matrix.md
**Tests**: required by the spec. Every scan and check has planted defects and a clean control.

## Phase 1: Setup

- [x] T001 Land spec, plan, research and matrix in `specs/010-release-readiness/`; recheck upstream #31; re-read research R1 against the current history
- [x] T002 Probes, outside the repository: the OpenTofu and provider pins, the provider fetched into an owned cache and then validated with no network, and where license and bundle metadata come from. Record them in `specs/010-release-readiness/research.md`
- [x] T003 [P] Write `LICENSE` with the approved MIT text

## Phase 2: User Story 1 - Where everything came from (Priority: P1)

**Goal**: one inventory by scope, with a check that fails on an unlisted third-party path.

**Independent Test**: `tools/release/inventory.sh` over the tracked tree, and a planted unlisted path.

- [x] T004 [US1] Write the inventory check `tools/release/inventory.sh` and its fixture test, failing first on a planted unlisted path, an unknown license shown as unresolved, and a bundled dependency missing from the built-artifact scope
- [x] T005 [US1] Rewrite `docs/provenance.md` by scope: source export, built artifacts including the image from spec 009, tools; code-generation inputs and output families; Go, npm and Debian enumerated; unknowns visible
- [x] T006 [US1] Bind the check in `quality/registry.textproto` and enroll its test in `tools/checks/required_tests.txt`

## Phase 3: User Story 2 - Nothing private ships (Priority: P1)

**Goal**: two scans over the whole export, distinct statuses, labels only.

**Independent Test**: planted defects in a scratch repository, in contents, names, history and metadata.

- [x] T007 [US2] Write `tools/release/neutrality.sh`, the generic scan over the whole export, with its fixture test failing first on a planted identifier in a current file, a deleted file and a commit message
- [x] T008 [US2] Add `--markers <file>`: labels, redacted locators, stated scope, literal absence, and the distinct `not-run` status with no file or an empty one. The fixture plants a marker in a file name, a directory, a blob, a message, a ref and an identity, and asserts its text appears in no output
- [x] T009 [US2] Bind both as checks in `quality/registry.textproto`: generic in the CI legs, private as `manual` and never required by CI; enroll the tests

## Phase 4: User Story 3 - Provisioning from a reviewed plan (Priority: P2)

**Goal**: one declarative target, validated offline, ruleset disabled.

**Independent Test**: after the fetch phase, `tofu fmt -check`, `validate` and `test` pass with no network and no credentials.

- [x] T010 [US3] Write `governance/github/`: one target, fresh isolated state, no imports, the ruleset disabled behind an activation input, pinned tools, and mock tests of owner, name and resource graph. A second target, an import block, another owner or name, or a changed graph fails a test
- [x] T011 [US3] Add the fetch-then-offline script `governance/github/offline.sh` and bind it as an opt-in registry stage, never in a required CI leg

## Phase 5: User Story 4 - The first hosted run is planned (Priority: P2)

**Goal**: the order, the gates and the first push's refusal, written down.

**Independent Test**: review against spec 001's gates and spec 007's refusal.

- [x] T012 [US4] Write `docs/release-runbook.md`: creation, first push, first green, ruleset activation; each operator gate; the first push's refusal and why no self-baseline follows

## Phase 6: Final scan, review and freeze

The final scan comes after everything it must cover: the inventory (T004 to T006), the governance target (T010, T011), the runbook (T012), and spec 009 complete. A candidate that differs from the scanned revision is refused.

- [x] T013 Private release scan at the frozen revision with the operator's marker file, mapping retained outside the repository; the result states scope and literal absence, or each finding has a decision. Record in `docs/evidence/m8.md` by label only
- [x] T014 Execute `docs/public-release-review.md` at that same revision; record reviewer and revision, with release gates shown pending
- [x] T015 `docs/evidence/m8.md`, `specs/README.md` statuses, the full verifier, freeze at the scanned revision, hold for independent acceptance. Accepted locally at `446e5c8` by an independent combined full
- [ ] T016 External release packet, outside the repository: configured identity, proposed release ref, the activation input, the open workflow choice. Technical readiness is recorded with publication pending


## Phase 7: User Story 5 - Public development on the reviewed core (Priority: P1)

**Goal**: one frozen exact candidate, an accurate README, reviewed decisions, the required check that exists.

**Independent Test**: the packet names the candidate; the governance witness fails on a context that names no job; the README matches the evidence records.

- [x] T017 [US5] (landed at `a42c9f1`) Relate the ruleset's required status check to the workflow's gate job and the CI-parity policy in `tools/release/governance.sh`, with the planted defects in `tools/release/governance_test.sh`: a context naming no job, a blank context, a renamed policy gate, a renamed workflow job; correct `governance/github/main.tf` to `gate`
- [ ] T018 [US5] Make `README.md` and the other public READMEs accurate for publication: implemented, independently accepted at a revision, hosted-unverified and unfinished states per milestone, the open M9 work visible as open, no production or hosted claim
- [ ] T019 [US5] Turn each pending packet item into a reviewed decision in the external packet: the commit identity, the holder line, each of the 15 unresolved licenses by scope (source-exported third-party content apart from fetched build and test dependencies and runtime artifacts)
- [ ] T020 [US5] Freeze one exact publication candidate after T017 and T018: name its revision, tree, export ref and holder in the packet; run the full verifier, both scans (private by opaque output) and the governance check at that revision; record them in `docs/evidence/m8.md`
- [ ] T021 [US5] After creation and first push through the approved plan: qualify the standard hosted Linux runners from real runs, recording cold, cached, queue, wall, concurrency and disk per leg in `docs/budgets/`; the first push exits 11 for lack of a base, the first real PR green and the activation follow the runbook


## Phase 8: User Story 6 - Distribution from the public repository (Priority: P2)

**Goal**: the image by digest in the repository-linked package, the artifacts as tag-bound downloads, verified anonymously.

**Independent Test**: the CI-parity policy declares the second workflow and its pinned structure; the publication is verified by a credentialless job.

- [x] T022 [US6] (a second policy of kind publish, `tools/ci/publish-policy.json`; twenty mutations) Extend `tools/ci/policy.json` and the CI-parity check to declare a second workflow `publish.yml`: trigger on tags of the trusted repository only, pinned action SHAs, `permissions` per job (`packages: write` and `contents: write` only where the job publishes, `{}` for the verifying job), no `pull_request_target`, no `workflow_run`, no cache; `//:ci_parity_test` fails on a drift
- [x] T023 [US6] (declared and witnessed offline; never run) Write `.github/workflows/publish.yml`: the full verifier at the tag, `bazel build //bff/cmd/bff:image` once, the digest read from the receipt's image measurement and compared with the built layout, `oci_push` to the repository-linked package with the job's token, the tarball, receipt, inventory and notices uploaded as release assets with a sha256 list; then a credentialless job that pulls by digest anonymously and checks the hashes
- [ ] T024 [US6] After an authorized publication: read back the package's public visibility, the anonymous pull and the asset hashes; record in `docs/evidence/m8.md`; a pre-release only, no stable-release claim

- T009: the generic scan is bound at `full`. The private scan is run at a freeze with the operator's marker file, never by CI, so `not-run` is its CI status by construction.
- T011: the governance test is a required Bazel test rather than an opt-in registry stage, because the sandbox gives it no network and the pinned zips give it no fetch, which is stronger than an opt-in stage.

## Dependencies

- T002 before T004, T010. T003 is independent.
- T004 before T005, T006. T007 before T008, T009. T010 before T011.
- T013 needs T006, T009, T011, T012 and spec 009's T013. T014 needs T013 and T003. T015 needs T014. T016 needs T015.

## Parallel Example

T003, T004, T007 and T010 touch different paths and can proceed together once T002 is done.

## Implementation Strategy

User Stories 1 and 2 first, because the final scan depends on both. The provisioning target is written once the probe says what offline validation can actually claim.
