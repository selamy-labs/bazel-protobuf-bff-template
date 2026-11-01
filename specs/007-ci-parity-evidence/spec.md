# Feature Specification: CI Parity, Base-Built Floors and Receipts (M6)

**Feature Branch**: `007-ci-parity-evidence`
**Created**: 2026-11-01
**Status**: Draft
**Parent**: [spec 001](../001-bff-template-foundation/spec.md) US4, FR-019, FR-021, FR-022, FR-024;
tasks T060–T070; T047's S005 re-witness. Builds on M5, accepted for the qualified local Linux profile
at `e0a12ac`.
**Excluded**: FR-023 and SC-007 (upstream #27), T067 (upstream #26), T068 (upstream #27). No shared
release exists, so these stay gaps with named owners. Hosted execution stays unobserved.
**Amends**: parent SC-001, see FR-022.

## Terms

- **Check**: a registry entry.
- **Leg**: one `--shard` run: `build-test`, `e2e` or `policy`. A CI job runs exactly one leg.
- **Owned check**: a `full` check with `shard` set. It runs in that leg and nowhere else.
- **Postcondition**: a `full` check with `postcondition: true`. It has no shard. It runs after the Bazel
  invocation of every leg with Bazel work, and judges that leg's log against that leg's obligations.
  Today these are `test-inventory` and `sandbox-evidence`.
- **Obligation**: the pair (check, leg) for an owned check, or (postcondition, Bazel leg). A target
  obligation is the pair (required test, leg).
- **Tested commit**: the commit a run judges: `HEAD`, or the pushed commit at pre-push.
- **Selection range**: what changed, from `--from` to `--to`. It chooses inputs and escalation. It never
  names a base.
- **Policy base**: the commit a tested commit is compared with: a strict ancestor of the tested commit,
  chosen by the caller. Naming a base says nothing about whether anyone reviewed it.
- **Anchor**: the independently reviewed commit that first contains the evaluator, or a consumer's first
  reviewed commit.
- **Coverage profile**: one entry of `quality/coverage_floors.json`: `language`, `metric`, `includes`,
  `excludes`, `floor`, with a reason per exclude.
- **Measured source set**: the tracked files in a tree that match a profile's `includes` and no `excludes`,
  computed from Git, never from a report.
- **Enrollment**: each tracked file of a language with a profile is in that language's measured source set or
  in an `excludes` entry. `includes` and `excludes` are judged per tree, so this needs no history.
- **Narrowing**: a surviving path, present in both trees, in the base's measured source set and absent from the
  proposal's. Adding a file to a tree is not narrowing, and neither is deleting one.

## User stories

**US1 (P1): One registry governs local and CI.** A maintainer changes a check once, and `full` and the
legs agree with no second list.
*Independent test*: remove a shard, duplicate an owner, mark a postcondition owned, drop a required test's
leg, or add an inline CI policy step. Each is rejected for its named reason, and the real workflow passes.

**US2 (P1): A proposal cannot authorize its own weakening.** A proposal lowers a floor, narrows a profile,
adds a suppression, edits the pattern set or replaces the evaluator. A checker built from the base still
fails it, and an unrelated change passes.
*Independent test*: real Git repositories with real base and proposal commits.

**US3 (P1): The aggregate cannot go green on missing work.** A skipped, cancelled, failed or absent leg
denies the gate. A failed leg rerun to success counts by its latest result, with its attempt recorded.
*Independent test*: the gate's own run block, executed over result fixtures.

**US4 (P2): Coverage is real and owned.** Floors come from language-native data over a declared source set,
for both modules, and only rise.
*Independent test*: a removed test lowers the measured number and fails; an empty report, a truncated
report, a missing module or a report beside a failed test all fail.

**US5 (P2): Claims stay separate.** A reader can tell offline, local, hosted, human and deployment evidence
apart.

## Functional requirements

Partition and postconditions

- **FR-001**: Each `full` check MUST be exactly one of: owned by one leg, or a postcondition. A check that is
  both, or neither, is exit 2. Without `--shard`, each check runs once, as today, and each postcondition
  judges the whole list.
- **FR-002**: The obligations MUST be derived from the registry and `required_tests.txt`, and be complete:
  each owned check in one leg; each postcondition in every Bazel leg and no other; each required test in
  exactly one Bazel leg. A leg's receipt MUST list its expected obligations and report each one, so a leg
  that skipped a postcondition cannot be PASSED.
- **FR-003**: The leg set in the workflow MUST equal the set derived from the registry. A leg removed from
  both a job and `needs` MUST be rejected. A union of check sets is not a sufficient test.
- **FR-004**: Each CI policy step MUST be a runner call for one leg. Inline policy commands are rejected.
- **FR-005**: `gate` MUST need every leg, and pass only if each result is `success`.

One Bazel invocation

- **FR-006**: A stage, or a leg, MUST issue at most one Bazel `test` or `coverage` invocation for its
  `KIND_BAZEL` checks, as M5's contract says. The verb MUST be `coverage` exactly when the selection
  includes `coverage-floor`, and `test` otherwise, so `pre-push` and `e2e` keep `test`.
- **FR-007**: That invocation MUST satisfy the test obligations and produce the coverage data. No native
  adapter may run `bazel test` or `bazel coverage` over repository targets.
- **FR-008**: Evaluator bootstrap is a `bazel build` of the evaluator's fixed target in a private base
  worktree and output base. It runs no test, judges nothing and is not a check invocation. Its failure MUST
  fail closed. Fixture checks that drive a scratch repository are unchanged.
- **FR-009**: Coverage tooling MUST run with no C++ toolchain and no JDK, like the rest of the template.
- **FR-010**: A helper's identity capture MUST read stdout only, under the existing bounded supervision, so a
  tool's warning cannot enter a path. On failure, a bounded part of stderr MUST reach the diagnostic. No parser
  picks a line out of mixed output.

Base and bootstrap

- **FR-011**: `floors-from-base` MUST run an evaluator built from a detached worktree of the base, against
  the tested tree. Nothing from the tested tree may build or configure that evaluator.
- **FR-012**: The policy base MUST come from a declared input: `--base=<rev>`, the hook's
  `bff-template.policy-base` setting, or CI's event value. `--from` is the selection range and MUST never
  be used as one. No default is derived from a branch name, `HEAD^` or anything else.
- **FR-013**: The runner MUST resolve the named revision once, and apply every test to the resolved SHA. These
  MUST give `floors-from-base` a refusal named for its real cause, and exit 11: no base named; malformed
  revision syntax; an unknown revision; shallow history; the same commit as the tested commit, however it was
  spelled; a commit that is not an ancestor; a base without the evaluator; a base that does not build;
  `--offline`.
- **FR-013a**: A different ancestor whose tree equals the tested tree MUST be accepted. No content changed, so it
  is a valid non-weakening comparison. Tree inequality proves nothing about review.
- **FR-014**: A refusal MUST NOT stop other checks. The receipt reports every other result, and cannot be
  PASSED. There is no flag, default or fallback that compares a tree with itself.
- **FR-015**: A receipt MUST record the resolved base SHA and how it was chosen: `argument`, `hook-config` or
  `ci-event`. It MUST NOT claim the base was reviewed. Review and readiness are separate records.
- **FR-015b**: `base_policy_digest` MUST use the base's own registry as the definition of its policy inputs. If that
  registry cannot be read or parsed, the identity is unavailable, and the receipt cannot be PASSED. This is metadata about
  the base, and no judgment of the proposal.
- **FR-015a**: A hook MUST read its policy base from the declared local setting, set by
  `//tools/hooks:install -- --policy-base=<rev>` and by nothing implicit. A zero `--from` or a policy change still
  escalates to `full`. With the setting, that push is judged against it. Without it, `floors-from-base` refuses. An
  ordinary push reads no base. A repository's first CI push is a separate case, which M8 handles explicitly.

Floors and coverage

- **FR-016**: Within coverage profiles only, these MUST fail against the base: a floor lowered; a profile
  removed; a `metric` or `language` changed; narrowing of the measured source set. A file newly added to the
  measured set counts in the percentage. A new profile is allowed.
- **FR-016a**: A tracked file of a profiled language that is neither measured nor excluded MUST fail, named as
  `unenrolled source`, by `check` and by the base-built `compare` alike. No move detection exists: a delete and add is judged as a deleted path and a new path, and the new
  path must be enrolled like any other.
- **FR-017**: Each file in the measured source set MUST appear in the language's report. An absent file MUST
  fail, and it MUST NOT count as covered or as not applicable. A file whose data counts nothing is absent, and a
  profile with no countable data fails rather than skipping its floor. An `excludes` entry MUST carry a reason from the
  allowed list. The evaluator enforces that a listed reason is present. Whether the reason is true of the file is a
  human review, and no tool here proves it. Executable code without data needs real data, or an explicit
  `unmeasured` boundary declared for review. Lacking a test is not a reason.
- **FR-018**: Suppression additions MUST be judged by the base's pattern set.
- **FR-019**: Coverage MUST be judged only after the Bazel invocation exits 0, and the check MUST receive that
  outcome from the runner, not infer it from summary lines. Any other exit with reports present is `FAILED`. An empty, truncated or unparsable report is `FAILED`.
- **FR-020**: Go floors MUST use **statement coverage** from the raw Go profile, labeled `statements`.
  rules_go's LCOV `DA`, `LF` and `LH` values MUST NOT be reported as Go's percentage. Go code reachable only as a
  subprocess has no statement data, so it has a separate profile, labeled `block-span-lines`. Go branch coverage
  is `unmeasured`, because the tool does not produce it. `unmeasured` is never `met` and never not
  applicable, and it is listed in the receipt and in capability states.
- **FR-020a**: TypeScript coverage is required, for code with real consumers. `web/client` MUST be measured from a
  real report, labeled `compiled-js-lines`, with branches shown apart. An empty or placeholder report MUST fail.
  Browser-only code is an explicit `unmeasured` boundary for review, and it is not counted as covered.
- **FR-020b**: Before any floor is set, the Go report and the raw profile MUST be reconciled, file by file.
- **FR-021**: Each of the root and quality modules MUST have its own profile, with the module's files
  demonstrably present in its report.

Usability, CI and documents

- **FR-022**: The canonical local command becomes `tools/verify.sh --base=<reviewed-sha>`. Parent SC-001, README,
  `fresh_clone.sh` and CI MUST say so. One command remains one command. `tools/verify.sh` with no base runs
  everything else and exits 11, naming what to pass.
- **FR-023**: A required test MUST have one leg in `required_tests.txt`, and the runner MUST judge each leg's
  inventory against it. New policy inputs MUST be enrolled in `escalate`, with real-registry known-base and
  digest cases.
- **FR-024**: A receipt MUST be uploaded per leg per attempt, under a unique name, with
  `if-no-files-found: error`, binding leg, revision, base revision, base policy digest, producer, attempt,
  terminal state, inventory and claim layer, with no personal identifier.
- **FR-025**: CI MUST use read-only permissions, no secrets, no private network and no credentials, MUST check
  out the claimed source, and fork proposals MUST behave the same.
- **FR-026**: Trigger scope and documentation MUST agree. The sentence "CI checks every ref" is narrowed.
- **FR-027**: A bypassed-hook commit MUST fail a CI-equivalent run. S005 spellings MUST be re-witnessed against
  the pinned tools where a tool exists (parent T047).

## Success criteria

- **SC-001**: Every row of the acceptance matrix has an observed positive and a named negative.
- **SC-002**: The final revision passes normal and hostile fresh-clone gates with `--base`, scoped and labeled.
- **SC-003**: Fixtures show a green gate is impossible with a missing, skipped, cancelled or failed leg.
- **SC-004**: The three flows in research R6 are reproduced exactly, with their negatives.
- **SC-005**: Evidence never claims hosted execution, required status or post-integration verification.

## Limits accepted

- A proposal controls its own copy of the workflow on a `pull_request` run. Base-built evaluation binds the
  evaluator, not the workflow that invokes it. See research R5.
- Local receipts name a base the caller chose. A named older base compares against older floors.
- FR-016 is the coverage profile contract and nothing wider. General weakening detection is upstream #26.
- Relocating a file by delete and add into a path that an existing `excludes` entry already matches is not detected.
  Entries are kept narrow and carry a reason, so a reviewer can see what they admit.
- No readiness verdict is computed here.
