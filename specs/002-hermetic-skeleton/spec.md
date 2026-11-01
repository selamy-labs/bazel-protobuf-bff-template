# Feature Specification: Hermetic Skeleton and Bootstrap (M1)

**Feature Branch**: `002-hermetic-skeleton`

**Created**: 2026-10-29

**Status**: Delivered. Current status is in the [index](../README.md)

**Input**: User description: "M1 hermetic skeleton and bootstrap … one command, verified pinned
toolchain, minimal honest verifier, separate versioned quality module, local CI workflow with an
aggregate gate, fresh-clone witness, measured cold/warm budgets; the preflight must detect missing
sandboxing with actionable diagnostics instead of silently weakening isolation. No application
code, no remote cache, nothing pushed."

**Parent**: [001 spec of specs](../001-bff-template-foundation/spec.md), milestone M1. This spec
covers parent tasks T001–T015 and parent requirements FR-025, FR-026 (measurement) and FR-028
(pins).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One command produces a verified, pinned toolchain (Priority: P1)

A developer whose machine has only the documented prerequisites clones the repository and runs one
command. Every tool that can affect a verdict is fetched at an exact, checksummed version. The
minimal verifier then runs and passes. The developer installs no language runtime, package manager
or scanner, and enters no credentials.

**Why this priority**: every later milestone builds on this. A template that needs extra host
setup is not a template.

**Independent Test**: from a fresh clone, with empty build caches and no credentials, the
bootstrap/verify command exits 0. It records the exact revision and the version of every resolved
tool.

**Acceptance Scenarios**:

1. **Given** a fresh clone and empty caches, **When** the developer runs the verify command, **Then**
   it fetches only pinned, checksummed artifacts, prompts for nothing, and passes.
2. **Given** a dependency declaration changed without updating the lock record, **When** verify runs,
   **Then** it fails, naming the lock and the regeneration command.
3. **Given** a legacy, non-module dependency file is added, **When** verify runs, **Then** it fails
   with the policy rule.
4. **Given** the vendored Spec Kit scaffold is edited by hand, **When** verify runs, **Then** it fails
   naming the drifted path and the pinned release.
5. **Given** a tool's recorded checksum does not match the downloaded artifact, **When** verify
   runs, **Then** it fails and does not use the artifact.

---

### User Story 2 - The minimal verifier catches real defects with actionable output (Priority: P1)

Before any application code exists, the verifier already enforces three guarantees: no committed
secrets, no inline suppression of checks, and no misspellings in tracked text. Each failure names
the check, the file and line, and the command to rerun it.

**Why this priority**: gates that arrive before code cannot be weakened later "just to get started".

**Independent Test**: each guarantee has a defect fixture that fails for the intended reason and a
corrected counterpart that passes.

**Acceptance Scenarios**:

1. **Given** a tracked file containing a credential-shaped secret, **When** verify runs, **Then** the
   `secrets` check fails, naming the file and line.
2. **Given** an inline lint-suppression directive in a source file, **When** verify runs, **Then**
   the `suppression-ban` check fails with a stable code.
3. **Given** a misspelled word in tracked text, **When** verify runs, **Then** `typos` fails, naming
   the word and the location.
4. **Given** each defect is corrected, **When** verify runs, **Then** it passes.

---

### User Story 3 - Missing isolation is detected, never silently weakened (Priority: P1)

The build sandboxes every action. When the host cannot provide namespace isolation, the preflight
and the build fail with the cause and the fixes. Ubuntu 24.04's default AppArmor policy (restricted
unprivileged user namespaces) is one such cause. The developer can only continue in a reduced
isolation mode by choosing it explicitly. Every result produced in that mode is labeled as having
weakened isolation and cannot count as full-strength evidence.

**Why this priority**: verified on the authoring host (2026-10-29). Without an explicit policy, the
build tool silently falls back to a sandbox with no namespace isolation.

**Independent Test**: on a host where isolation is unavailable, the default verify command fails
with the diagnostic. The explicit reduced mode passes, and its receipt is labeled
`isolation: weakened`. On a capable host, the default passes with `isolation: full`.

**Acceptance Scenarios**:

1. **Given** a host where unprivileged user namespaces are restricted, **When** the preflight runs,
   **Then** it exits nonzero. It names the restriction, gives at least two remediation options
   (and which of them need administrator rights) and gives the explicit opt-in command.
2. **Given** the same host, **When** the default verify command runs, **Then** it fails rather than
   falling back to weaker isolation.
3. **Given** the explicit reduced-isolation opt-in, **When** verify runs, **Then** it passes and its
   receipt and summary state `isolation: weakened`.
4. **Given** a capable host, **When** verify runs by default, **Then** its receipt states
   `isolation: full`.

---

### User Story 4 - CI runs the same verifier at the exact revision (Priority: P2)

A maintainer reads one CI workflow definition. It checks out the exact commit under test, runs the
same verify command as local development, and reports a single aggregate verdict. That verdict
fails if any required job is missing, skipped, cancelled or failed.

**Why this priority**: local hooks are bypassable; only CI is evidence. The workflow is authored
and linted locally in M1 and runs once a remote exists (M8).

**Independent Test**: the workflow passes workflow linting and security linting. A parity test
confirms its policy step is exactly the verify command. Aggregate-gate fixtures produce the
documented verdicts.

**Acceptance Scenarios**:

1. **Given** the workflow, **When** it is linted, **Then** there are no findings. Actions are pinned
   by immutable digest, permissions are read-only, and credentials are not persisted.
2. **Given** a workflow step that runs a policy command other than the verify command, **When** the
   parity test runs, **Then** it fails.
3. **Given** a required job result of `skipped`, `cancelled` or `failure`, **When** the aggregate
   gate evaluates, **Then** it fails.

---

### User Story 5 - Feedback cost is measured, not asserted (Priority: P3)

A maintainer can see how long bootstrap and verify take, cold and warm, and how much memory they
use, on a named host profile. The figures are recorded with the revision and the isolation mode.

**Why this priority**: budgets in later milestones derive from these measurements.

**Independent Test**: the measurement command produces a record with p50/p95 wall time and peak
memory for each scenario, with host details, revision and isolation mode.

**Acceptance Scenarios**:

1. **Given** the measurement command, **When** it runs with N repetitions, **Then** it writes a
   record containing cold and warm figures for bootstrap and verify.
2. **Given** a record produced in reduced isolation, **When** it is read, **Then** it is labeled
   `isolation: weakened`.

### Edge Cases

- Shallow clone, or a clone without the base revision: Spec Kit and lock checks need no history and
  still work. Checks that need history are out of scope for M1.
- Offline after the first fetch: verify passes from caches. A first fetch with no network fails
  with a clear network message, never a partial pass.
- The checkout path contains spaces: bootstrap and verify still work.
- Running inside a Git worktree: the caches do not collide between worktrees.
- A tool artifact is unavailable upstream: fail with the artifact name and pinned URL. Never fall
  back to "latest".
- A non-Linux host (macOS): the documented secondary profile. Its isolation mode is reported as
  the platform default and is measured separately.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The repository MUST declare one build tool version, and every dependency through a
  module system with a committed lock record. A lock mismatch MUST fail.
- **FR-002**: Legacy, non-module dependency declarations MUST be rejected.
- **FR-003**: Bazel, the Go toolchain and the native tools (secret scanner, spell checker, shell
  linter, workflow linter, workflow security linter, build-file formatter, YAML processor, Python
  runner) MUST be fetched at exact versions with recorded checksums. Each tool
  arrives with its first real consumer: the JavaScript toolchain and package manager are pinned
  in M2 with TypeScript code generation, and the hook manager in M5 with the hooks.
- **FR-004**: One documented command MUST bootstrap and verify a fresh clone, with no prompts, no
  credentials and no host installs beyond the documented prerequisites.
- **FR-005**: The verifier MUST include the guarantees `bzlmod-policy`, `speckit-scaffold`,
  `secrets`, `suppression-ban` and `typos`. Each MUST have a defect fixture that fails and a
  corrected fixture that passes.
- **FR-006**: Every failure MUST exit nonzero and print the check id, the location, a bounded
  message and a rerun command.
- **FR-007**: The verifier MUST use namespace-isolated sandboxing by default and MUST fail when it
  is unavailable. Reduced isolation MUST require an explicit, named opt-in and MUST be recorded in
  every receipt and summary it produces.
- **FR-008**: A preflight MUST detect the missing prerequisites (Git, bash, curl, coreutils),
  unavailable isolation and insufficient disk space. For each it MUST print the cause and the
  remediation, and state which remediations need administrator rights.
- **FR-009**: Shared quality checks MUST live in a separately versioned module (the
  `bff_template_quality` module at version 0.1.0). The application root MUST consume it through a
  local override. The quality module MUST NOT depend on the application.
- **FR-010**: A CI workflow MUST check out the exact revision, persist no credentials, use
  read-only permissions, pin actions by digest, run only the verify command as policy, and
  aggregate the job results into one required gate.
- **FR-011**: A fresh-clone witness MUST clone the current revision into an empty location, with
  empty caches and no credentials, run verify, and record the revision, the outcome and the
  isolation mode.
- **FR-012**: A measurement harness MUST record p50/p95 wall time and peak memory separately for
  bootstrap, the first verify after bootstrap, an unchanged warm verify and a verify after a source
  edit. Each record MUST carry the host profile, the exact revision and the isolation mode.
- **FR-013**: The Spec Kit scaffold MUST be checked in two ways. Offline, every vendored file is
  compared against a committed hash manifest. With network access, the scaffold and the manifest
  are regenerated from the pinned release using a pinned runner, a pinned Python and hash-locked
  dependencies, and that provenance check is required in CI. See research R4 for the stated limit
  of the offline check.
- **FR-014**: No application code, remote cache configuration or remote-service credentials are
  introduced in M1.

### Key Entities

- **Tool pin**: name, version, source URL, checksum, platform.
- **Verify receipt**: revision, isolation mode, the checks run with their results, the tool
  versions, start and end times.
- **Host profile**: OS, CPU count, memory, disk free, isolation capability.
- **Budget record**: scenario, cold or warm, N, p50, p95, peak memory, host profile, revision,
  isolation mode.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On a capable host, a fresh clone with empty caches reaches a passing verification
  with one command, zero prompts and zero credentials.
- **SC-002**: 100% of the defect fixtures (5 guarantees) fail for their intended reason, and 100% of
  their corrected counterparts pass.
- **SC-003**: On a host without isolation, 100% of default verify runs fail with the isolation
  diagnostic, and 0% silently pass in a weaker mode.
- **SC-004**: Cold and warm figures for bootstrap and verify are recorded for at least one host
  profile, with N ≥ 5 warm runs.
- **SC-005**: A verify run with downloads disabled succeeds after a first success, without Bazel
  fetching anything, and completes in under half the cold time on the same host. This run skips the
  networked provenance check and is not a claim of network isolation.

## Assumptions

- **A1 (validated 2026-10-29)**: The authoring host (Ubuntu 24.04,
  `kernel.apparmor_restrict_unprivileged_userns=1`) cannot run the namespace sandbox. The probe
  failed with `mount: Permission denied` inside the sandbox helper, and the build tool did not
  register the namespace strategy. M1 evidence from this host is therefore produced in the explicit
  reduced-isolation mode and labeled that way. Full-isolation evidence is expected from CI, where
  the disposable runner can lift the restriction declaratively.
- **A2**: Hosted Linux CI runners allow lifting the restriction with an administrator command on the
  disposable VM. This is to be confirmed on the first hosted run (M8, when a remote exists). Until
  then the CI full-isolation claim is unverified.
- **A3**: Tool versions follow the parent research R-03, with releases at least 14 days old when
  pinned.
- **A4 (deviation from parent task order)**: Parent task T005 (the JavaScript toolchain) moves to M2.
  In M1 it would have no consumer except a smoke target, which constitution I forbids. Go stays in
  M1 because the `suppression-ban` check is written in Go and is the first code in the quality
  module.
