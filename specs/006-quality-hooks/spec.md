# Feature Specification: Quality Registry, Runner and Lifecycle Hooks (M5)

**Feature Branch**: `006-quality-hooks`
**Created**: 2026-11-01
**Status**: Delivered. Current status is in the [index](../README.md)
**Parent**: [spec 001](../001-bff-template-foundation/spec.md) US3, FR-013–FR-020, SC-005, SC-006;
[check registry](../001-bff-template-foundation/contracts/check-registry.md) and
[hook stages](../001-bff-template-foundation/contracts/hook-stages.md) contracts.

## User Scenarios & Testing

### User Story 1 - One rule, one definition (P1)

A maintainer changes a quality rule in one place. Hooks, the full verifier and CI then use it at the
same pinned tool versions.

**Independent Test**: delete a rule from the verifier's expected set by editing only the registry,
and the full run no longer reports it; a malformed registry stops every stage with a configuration
error.

**Acceptance Scenarios**:

1. **Given** the registry, **When** any stage runs, **Then** the checks it runs are exactly the
   registry's entries for that stage.
2. **Given** a duplicate id, a check missing `full`, a failing `SIGNAL`, a `GUARANTEE` without a
   defect witness or a missing class, **When** the registry loads, **Then** the run exits 2.

### User Story 2 - Fast, faithful commit feedback (P1)

A developer or agent commits. Cheap content checks read exactly what will be committed, and a
failure names the check, the file and the rerun command.

**Independent Test**: in a temporary repository, stage a bad change beside an unrelated unstaged
edit. The commit fails for the intended reason and the unstaged edit is byte-identical afterward.

**Acceptance Scenarios**:

1. **Given** a staged secret, banned suppression or malformed schema, **When** committing, **Then**
   the commit is rejected, naming the check, path and rerun command.
2. **Given** `git commit -a` or `git commit <path>`, **When** the hook runs, **Then** it reads the
   index Git is about to commit, not the main index.
3. **Given** partial staging, untracked files, renames, deletions and paths with spaces or newlines,
   **When** the hook runs, **Then** only staged content is read and the working tree and index are
   unchanged.

### User Story 3 - Push checks the commit being pushed (P1)

A push runs broader checks against the pushed commit in a separate reusable worktree, so a clean or
dirty working tree cannot change the verdict.

**Independent Test**: push a commit that breaks a test while the working tree is clean; the push is
rejected; the corrected commit pushes.

**Acceptance Scenarios**:

1. **Given** a pushed commit that breaks a test, **When** the hook runs, **Then** the push is
   rejected and a working-tree edit cannot rescue it.
2. **Given** a change to policy files, **When** pushing, **Then** the stage escalates to `full`.
3. **Given** a first push with no remote base, **When** pushing, **Then** it escalates to `full`.

### User Story 4 - Bounded, owned execution (P1)

A hook or verifier cannot hang, leak a process that keeps capacity, or wait on input.

**Independent Test**: run a check that ignores signals, starts a `setsid` child and reads stdin. It
is reported `TIMED_OUT`, the child is gone and the run exits 10.

**Acceptance Scenarios**:

1. **Given** a deadline passes during Bazel, **When** the runner stops it, **Then** the client it
   started is interrupted and gone. A server stops only on an output base this run holds exclusively,
   after its exact identity is proved.
2. **Given** a daemon child outlives a foreground command, **When** the command exits, **Then** its
   slot is free.
3. **Given** active foreground work, **When** another run asks for that slot, **Then** it waits
   within its bound and never runs alongside it.
4. **Given** an independent producer is using the checkout's Bazel server, **When** this runner's
   deadline passes while it waits for that server, **Then** the other work continues and finishes,
   and the server is the same process afterward.

### User Story 5 - Installation is safe and repeatable (P2)

**Acceptance Scenarios**:

1. **Given** a foreign hook, **When** installing, **Then** it is preserved and still runs.
2. **Given** hooks installed twice, **When** comparing, **Then** the result is identical.
3. **Given** a linked worktree or `core.hooksPath`, **When** installing, **Then** the hooks land
   where Git reads them.
4. **Given** the build outputs are cleaned, **When** the hook runs, **Then** it still runs the
   pinned prek, not one found on `PATH`.

### User Story 6 - Go analyzers run (P2)

**Acceptance Scenarios**:

1. **Given** a vet or staticcheck finding in repository Go code, **When** building, **Then** the
   build fails naming it; the corrected code builds.

### Edge Cases

- Staged symlinks, gitlinks and conflicted entries.
- A deletion-only commit has no content to check and passes.
- A multi-ref push evaluates only the first ref (a limit of prek; CI covers the rest).
- A pushed commit without `//:quality` fails with a configuration error, never a pass.
- Concurrent pushes in one repository share one worktree and must not interleave.
- A hook interrupted with Ctrl-C leaves no running child.

## Requirements

- **FR-001**: The registry MUST be the only list of checks. Hooks and `tools/verify.sh` MUST select
  from it; no stage may carry its own list.
- **FR-002**: The registry MUST be a schema-typed file validated at load and in a test, with stable
  ids, stages, class, kind, timeout, selector and witness.
- **FR-003**: Adapters MUST only translate file sets and invocation.
- **FR-004**: One runner MUST execute every stage. All `bazel` checks in a stage MUST run in one
  `bazel` invocation, beyond the one that starts the runner.
- **FR-005**: `native` checks MUST run in a bounded pool, each with a deadline, closed stdin and an
  owned process tree. Surviving descendants MUST be removed.
- **FR-006**: The runner MUST use the contract's exit codes and translate Bazel's codes explicitly.
- **FR-007**: Diagnostics MUST follow the contract's line format, bounded to 50 lines per check, and
  every failure MUST name a rerun command. Output MUST also be available as JSON.
- **FR-008**: A `GUARANTEE` check with zero inputs at `full` MUST exit 12 unless it has `allow_empty`.
- **FR-009**: Every check at pre-commit MUST read blobs from the index Git names, so untracked and
  unstaged content cannot change a check's verdict, and pre-commit MUST NOT write to the index or
  working tree. Pre-commit MUST stay a fast subset distinct from `full`.
- **FR-009a**: The runner is built from the checkout before any check exists to run, so a broken file
  in the runner's own dependency graph can stop startup. That outcome MUST be a separate bootstrap
  `ERROR` that says no check ran. It blocks the commit and MUST NOT be reported or counted as a
  rejection of staged content.
- **FR-010**: Pre-push MUST judge the pushed commit in a detached reusable worktree with its own
  output base, using the policy of that commit.
- **FR-011**: A change to policy inputs MUST escalate pre-push to `full`. Pre-commit MUST NOT escalate.
- **FR-012**: Paths MUST stay NUL-delimited end to end.
- **FR-013**: Concurrency capacity MUST be held by locks in the waiting parent, not inherited by any
  child, and acquisition MUST be bounded.
- **FR-014**: The runner MUST signal only the Bazel client it started. It MUST NOT stop a Bazel server
  unless it holds an isolated output base exclusively and has proved the exact process belongs to
  that output base; a PID file or path alone proves neither. A server it does not own MUST be left
  running and reported. The push worktree's owned server has a short idle bound.
- **FR-015**: Hooks MUST be pinned prek, one local hook per stage, with `pass_filenames = false` and
  installation that is idempotent, preserves foreign hooks, and honors worktrees and `core.hooksPath`.
- **FR-016**: Go analyzers MUST fail the build on a finding, under the pinned Go SDK.
- **FR-017**: TypeScript MUST be linted and format-checked with pinned tools, and the suppression
  ban's ESLint spellings MUST be re-witnessed against that pinned ESLint.
- **FR-018**: Every `GUARANTEE` MUST have a named defect case and a corrected case.
- **FR-019**: Stage cost MUST be measured cold and warm, including edited input, and recorded with
  its isolation label. A budget MUST NOT be declared without a measurement.
- **FR-020**: The two M4 lessons MUST have standing witnesses in `full`: automatic toolchain discovery
  must not change Go linking, and the browser's closure includes its fonts and configuration.
- **FR-021**: Bypass MUST be documented accurately, and a bypassed commit MUST fail `full`.
- **FR-022**: Guarantees and review signals MUST be classified, with the human-review list documented.

## Key Entities

See the parent [data model](../001-bff-template-foundation/data-model.md): Check, Registry, Stage
run, Result, Budget.

## Success Criteria

- **SC-001**: Every fixture in the hook contract passes, each failing for its named reason.
- **SC-002**: Unstaged content is byte-identical after 20 of 20 partial-staging runs.
- **SC-003**: A check that hangs, ignores signals and forks a `setsid` child ends within its deadline
  plus 15 seconds, with nothing left running.
- **SC-004**: Warm pre-commit cost is measured at 1, 50 and 500 staged files, and pre-push on a
  one-package edit; the record says whether each provisional target (5 s, 90 s) was met.
- **SC-005**: The full run reports every registry entry and the existing required-test inventory.

## Assumptions

- `quality/` is a separate Bazel module, so `//quality:run` cannot exist. The entry point is
  `//:quality`, wrapping `@bff_template_quality//runner`. The contract is updated.
- Upstream #25 and TS #20 are open and unreleased; no revision can be cited yet. This repository
  follows the contract recorded here and adopts upstream's when it is released.
- Coverage (S005 re-witness, `coverage-floor`), `floors-from-base` and `--shard` belong to M6.
- `complexity`/`duplication` are reviewed here and not adopted; `generated-fresh` has no committed
  generated files to judge. `contract-consumers` remains upstream.
- macOS runs the process-group path only and stays unverified.
