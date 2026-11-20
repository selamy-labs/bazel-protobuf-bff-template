# Implementation Plan: CI Parity, Base-Built Floors and Receipts (M6)

**Branch**: `007-ci-parity-evidence` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

The registry's checks split into three legs and shared postconditions, so CI runs the same policy as
`full`. One Bazel invocation gives test results and coverage. Coverage floors and suppression patterns are judged by
an evaluator built from a caller-chosen base. One writer, small traced commits, no remote and no hosted run.

## Shape

- `quality/proto/registry.proto`, `quality/registry.textproto`: `shard`, `postcondition`, validated. Legs and obligations derived.
- `quality/runner`: `--shard`, `--base`; verb selection in `PlanBazel`; CI receipt fields; leg inventory; stdout-only `capture`.
- `quality/checks/coverage`: pure Go merger for the output and report generator flags, keeping the raw Go profile.
- `quality/checks/floors`: the evaluator: profiles, measured source sets, narrowing, floors, suppression patterns.
- `tools/policy/from_base.sh`: a detached base worktree, a private output base, one `bazel build`. Reuses the push worktree's custody:
  exclusive lock, process handle, no shared server.
- `quality/coverage_floors.json`: under `^quality/`, so it escalates. `.bazelrc` and `bazelrc.allow` gain the `coverage` lines.
- `.github/workflows/ci.yml`, `tools/ci/policy.json`, `quality/checks/ciparity`, `tools/ci/mutations.tsv`.
- `tools/verify.sh`, `tools/bootstrap/fresh_clone.sh`, README, parent SC-001: `--base`.
- `tools/hooks/install.sh`, `tools/hooks/stage.sh`: the declared `bff-template.policy-base` setting.
- `docs/quality/evidence.md`, capability states, classification, contracts.

## Stages

1. Spec, research, plan, tasks and matrix committed. Remaining probes T002, then adjust the plan.
2. Stdout-only capture, red first. Legs, obligations and parity. `full` unchanged until the partition test passes.
3. Workflow legs, receipts, gate fixtures.
4. Coverage verb and merger: measured, reported, no floor.
5. `--base` and the hook setting. **Anchor commit**: evaluator, first floors, `floors-from-base` registered. Record the refusal at the
   anchor. Hold for its own independent targeted review. No source approves itself.
6. Descendant commits use it: base through `fresh_clone.sh` and CI, flows R6 reproduced.
7. Evidence, ledgers, cheap checks, freeze, normal and hostile gates with `--base`, measure once, hold for independent acceptance.

M7 then needs M6's acceptance, and a real separate neutral consumer with a canonical-pin upgrade. M8 needs the R6 re-anchor note, the
explicit first-push CI case, and the provisioning plan offline.

## Constitution and gates

- Upstream contracts stay upstream. The evaluator judges coverage profiles and suppression patterns only. It is not a policy-diff, and it
  adds no issuers, no readiness and no branch-protection change.
- No self-baseline path exists in any form: no flag, default or environment variable.
- Closed throughout: origin, publication, settings, Apps, access, apply, destroy, merge, paid runs.
- Mutants run in exclusive copies. A setup failure earns no credit. Red before code is claimed only where it happened.

## Risks

| Risk | Handling |
| --- | --- |
| `nogo` differs under instrumentation | T002 plants a vet defect under `coverage` before relying on it |
| rules_go's `.cover` location moves at a bump | Pin fixture; a missing profile fails |
| Instrumented data binaries change another tool's output | FR-010, plus 36 of 36 under `coverage` at each pin change |
| Bare `tools/verify.sh` now exits 11 | Message prints the fix; README, SC-001 and `fresh_clone.sh` move together |
| Base worktree inherits tested-tree config | Builds from the base's `.bazelrc` and `MODULE.bazel`; a mutant makes the tested tree's copy break it |
| A second private output base doubles disk and memory | Measure; keep the capacity slot |
| TS coverage cannot be measured natively | A mature tool under the 14-day pin rule, with provenance. `unmeasured` is not an end state |

## Bounded failure-log retention (T028)

The existing quality runner prepares a fresh private artifact from the current receipt and its explicitly referenced logs. Its export uses the current registry only to constrain check IDs and copy counts; it does not certify evidence claims or replace upstream receipt validation. A root-confined reader refuses symbolic links, non-regular files, foreign/unmarked directories and files beyond the existing retention bound. Selection is limited to the single marked run directory and exact check filenames. The original receipt bytes and failure state remain intact; unrelated files are neither copied nor removed. CI uploads only the prepared directory, not a naming glob.

The stage retains bounded combined Bazel output independently of its concise terminal summary, so ordinary target assertion text survives. A real runner/fake-Bazel fixture fails a named test, exports that receipt, and reads the assertion back beside an excluded same-prefix foreign sentinel. Passing receipt-only, malformed/foreign/symlink/outside/oversize reference cases and closed workflow input/step-order mutations cover the export boundary. Required verifier/gate, tool pins, permissions and retention remain unchanged.

## Native stage execution follow-through (T029)

Preserve the existing stage, check, supervisor and policy model. Give exclusivity precedence when classifying native plans, leaving exclusive execution after both pools and the stage Bazel invocation. For current-tree native helpers only, introduce `{bazel_client}` as a maintained invocation boundary preserving the pinned Bazel startup vector and the helper's original arguments. A shell thunk executed under the same native subreaper is insufficient on its own: a cold default daemon can still be adopted/reaped. Resolve client execution outside indiscriminate native descendant cleanup while preserving native-child cleanup and bounded stage/check cancellation; use existing client-only RunBazel semantics rather than a process-name exemption or operator prewarm. Keep `{bazel}` unchanged for base-evaluator relative paths; never route base builds through a proposal wrapper. Use literal POSIX argument quoting with exec, not evaluation. Change the two actual image/vulnerability consumers without changing their target, network, isolation or gate definitions.

Witness supervised native helpers consuming the production registry command definitions: omitted startup arguments fail the requested output-base/vector assertion; empty startup and relative base-wrapper controls remain valid. A held critical section detects overlapping exclusive post-Bazel checks. Retain old-source RED and corrected GREEN, whole runner regression and unchanged full qualification; do not claim the earlier hosted failure cause or infer a source correction from warm-server preflight.

The three retained postmerge FUTURE REVISE reports additionally require a cold-start/default plus unrelated-producer survival witness and a positive ordinary bounded-pool control. The current argument/critical-section draft alone does not cover those lifetime invariants. No production correction or complete causal acceptance is claimed before those controls and the invocation boundary are implemented and run.


T029 uses a parent-owned per-check native Bazel client channel, outside the check subreaper. The base-relative `{bazel}` contract is unchanged. A single check-owned terminal admission mutex covers accepted-connection registration and process start; teardown marks terminal before closing the listener/accepted connection, cancellation and drain. The existing client-only interrupt/grace owner remains shared beneath split stdout/stderr execution, preserving cquery pathname output. A bounded length-framed protocol preserves raw argument bytes and empty arguments, refuses missing terminal status/partial protocol/output-write failures, and closes blocked I/O on completion/deadline/disconnect. One active connection per check bounds server-side concurrency; ordinary stage pooling remains. Private transport limits are 16MiB argument bytes,1,048,576 arguments,32KiB output frames and1,048,576 frames, with host exec admission still applying to requests, with 10-second client grace plus 1-second pipe-drain bound; no quality threshold, tool pin or daemon-stop authority changes. The short private0700 socket directory is owned and removed by that check. Required controls cover separate simultaneous streams, pending decode, admission/start/stop races, late invocation, unread peers/full output, truncation, failed writes and overlapping cancellation, in addition to the actual cold-daemon witness and existing leaked-child cleanup.

T029 frames have one shared whole-frame writer/failure latch and final status follows both stopped output loops. Explicit parent-owned stdout/stderr pipe pairs separate the single process waiter from output copying; channel/pipe-reader close releases blocked copying, with a one-second post-exit drain timeout reported as ErrWaitDelay. WaitDelay alone cannot bound a blocked socket Writer. An independent reader observes peer lifetime even during output-free client execution; request half-close is a failure. The entrypoint acknowledges status only after successful local output writes and status validation, then waits for parent EOF; the parent completes bookkeeping before closing that connection. Preparation/protocol/transport/copy/drain/incomplete-request errors remain parent-owned and sticky after helper exit0, while ordinary Bazel exit codes remain interpretable. Required controls include suppressed proxy error beside helper0, zero-exit process with incomplete output, forced frame competition, silent disconnect, late/partial calls and legitimate interpreted nonzero client exit.
