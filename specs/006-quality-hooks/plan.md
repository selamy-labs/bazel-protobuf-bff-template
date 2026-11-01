# Implementation Plan: Quality Registry, Runner and Lifecycle Hooks (M5)

**Branch**: `006-quality-hooks` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

A typed registry defines every check. One Go runner executes a stage from it: staged blobs at
pre-commit, the pushed commit in a separate worktree at pre-push, everything at `full`.
`tools/verify.sh` keeps its scrubbed environment and preflight and hands the rest to the runner, so
its hand-written check list goes away. Pinned prek wires the stages into Git. Go analyzers and
TypeScript lint arrive here, because the registry has to judge them.

## Technical Context

| Item | Choice |
| --- | --- |
| Language | Go 1.27.2 for the runner, shell for thin adapters |
| New pins | prek 0.5.5, x/tools 0.52.0, staticcheck 0.8.2, ESLint 10.13.0, typescript-eslint 8.71.2, Prettier 3.9.10 |
| New quality dependency | protobuf-go 1.36.12 (R4) |
| Testing | `go_test` with a fake Bazel; `sh_test` hook fixtures with real Git and prek; one real-Bazel hook witness in `full` |
| Constraints | Runner never writes the index or working tree; deadlines on everything; no new service |

## Constitution Check

| Principle | Status | Notes |
| --- | --- | --- |
| I. Real slice | PASS | Every check judges real inputs. Entries with nothing to judge are not registered (R11). |
| II. Contracts first | PASS | Follows the two parent contracts; deviations are written back to them. |
| III. Hermetic, pinned | PASS | New tools are pinned by digest or lock. The shim's `PATH` fallback is closed by the installer (R2). |
| IV. One policy | PASS | This milestone is that principle. |
| V. Cannot weaken itself | PASS (scoped) | M6 owns base-built evaluators. Bypassable hooks are not a security boundary. |
| VI. Exact-revision evidence | PASS | Receipts keep every field; pre-push evidence names the pushed commit. |
| VII. Guarantees vs signals | PASS | Classification is a registry field and a document. |

## Project Structure

```text
quality/
  proto/registry.proto        schema
  registry.textproto          the one list of checks
  adapters/native.sh          tool invocations, moved from tools/checks
  runner/                     registry, select, snapshot, exec, supervise, slots, report, main
  analysis/                   nogo definition
tools/
  verify.sh                   scrub and preflight; then the runner
  hooks/                      stage.sh, install, fixtures
prek.toml                     one local hook per stage
```

Deleted by adoption: `tools/checks/verify_body.sh`, `inventory.sh`, `tool_paths.sh` and their tests,
each replaced by a Go or fixture witness that covers the same cases.

## Delivery order

Tooling that changes verdicts lands first (T002–T005), so each is visible to the existing verifier.
The runner is then built beside the old body (T006–T010), and a single commit switches `verify.sh`
to it and deletes the old body (T011). Hooks follow (T012–T014). The closing tasks measure and record
(T015–T018).

## Risks

| Risk | Control |
| --- | --- |
| Reordering checks hides a regression | T011 keeps every id's witness and the 32-target inventory, and `fresh_clone.sh` reruns normal and hostile |
| A nested `bazel run` deadlocks | R5, witnessed by the first full run |
| Cold pre-push is slow | Its own output base shares only the repository cache; measured, not hidden |
| A pushed old commit has no `//:quality` | Configuration error, exit 2 |
| Real-Bazel fixture time inflates `full` | Measured in T017; the fixture is a small repository |

## Not in M5

CI shards, base-built floors, coverage (M6); a separate consumer (M7); provenance inventory and
release plan (M8). Upstream hook adapters, policy-diff, evidence validators, the consumer analyzer
and worker runtime (#25–#29, TS #20) stay upstream.
