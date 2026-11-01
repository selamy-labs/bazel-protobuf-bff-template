# Research: Quality Registry, Runner and Lifecycle Hooks (M5)

**Date**: 2026-11-01. The 14-day pin rule therefore admits releases from 2026-10-18 or earlier. Probes
below ran in scratch clones and temporary repositories; each result is a witness for the design, not
for the final code.

## R1 Go analyzers (T091)

The diagnosis in spec 004 R7 named rules_go, and that was too narrow.

- **Rechecked with cgo off.** nogo under rules_go 0.64.2 and Go 1.27.2 still fails on every package:
  `export data version 5 is greater than maximum supported version 4`. Removing cgo did not change it.
- **Cause.** nogo reads export data with `golang.org/x/tools`. rules_go 0.64.2 selects v0.48.0, whose
  `pkgbits` stops at V4. Go 1.27 writes V5. v0.52.0 (2026-10-10) has V5.
- **Route.** `go_deps.module` raises x/tools to v0.52.0 with its sum. Go module selection applies
  across modules, so rules_go's own `org_golang_x_tools` follows. rules_go and the Go SDK stay
  pinned, so this does not need 0.65.0 and does not wait for 2026-11-05.
- **Probe.** With this one change, nogo with vet built `//bff/...`. A planted
  `fmt.Sprintf("%d", "not a number")` failed the build with the `printf` finding, and removing it
  built again.
- **Limit.** This pairs a newer analysis library with an older rules_go. It is re-judged when rules_go
  moves.

staticcheck v0.8.2 (2026-10-12) passes the pin rule and is the second half of T091. It runs on
repository code only, because third-party findings are not this repository's to fix.

## R2 prek (T056, T057)

v0.5.5 (2026-10-05) is the newest release that passes the pin rule; v0.5.6 is one day too new. It is
pinned through the multitool lock: musl builds on Linux, so it adds nothing to the glibc floor. The
digests come from release metadata, and the Linux x86_64 archive was re-hashed to match.

Probed with the pinned binary in temporary repositories:

| Behavior | Observed | Consequence |
| --- | --- | --- |
| `git commit -a` | `GIT_INDEX_FILE=<abs>/.git/index.lock` | **A scrubbed environment would read the wrong index.** Pre-commit keeps it |
| `git commit <path>` | `GIT_INDEX_FILE=<abs>/.git/next-index-N.lock` | Same. `-m` alone gives a relative `.git/index`, resolved before any `cd` |
| Installed shim | Absolute path to prek, falling back to `prek` on `PATH` when it is missing | **A pin in Bazel's outputs silently stops being a pin after `bazel clean`.** The installer copies the verified binary under the Git common directory |
| Foreign hook | Moved to `.legacy`, run first, also after a second install | A foreign hook with no shebang fails every commit with `Exec format error`. Documented |
| Partial staging | prek's own patch stash; the working tree is restored afterward | Fixture checks bytes |
| `core.hooksPath` | Honored by `install` | Fixture |
| Linked worktree | `git rev-parse --git-path hooks` is the common hooks directory | Fixture |
| Malformed `prek.toml` | Parse error, commit refused | Fixture |
| Pre-push update | `PRE_COMMIT_FROM_REF` and `PRE_COMMIT_TO_REF` set | |
| Pre-push, first ref to an empty remote | `FROM_REF` absent | Treated as unknown base: escalate |
| Pre-push, new branch | `FROM_REF` is the merge base | |
| `git push origin main two` | One hook run, for `main` only | **Confirmed limit.** The second ref is unchecked locally; CI covers it |
| Ref deletion | The hook does not run | Nothing to judge |
| stdin at the entry | Empty | prek consumes it, so the runner cannot see other refs |

Upstream #25 and TS #20 are open with no release, so T056 has no revision to cite. The configuration
says so.

## R3 Entry point

`quality/` holds its own `MODULE.bazel`, which makes it a repository boundary, so `//quality:run`
cannot exist. `//:quality` is a thin root target over `@bff_template_quality//runner`. It passes the
repository's pinned tools as data, so native checks need no second Bazel invocation.

## R4 Registry format

Textproto with a proto schema, as agreed. The cost is real: the quality module gains protobuf-go and
the `buf_generate` rule moves into it. The benefit is a typed registry that rejects unknown fields
with no parser of ours. The rule takes the schema import as an attribute, because the quality module
must not depend on root's `@protovalidate_proto`.

## R5 Process ownership

- **Subreaper scope.** `PR_SET_CHILD_SUBREAPER` on the main runner would adopt the Bazel server
  whenever the runner is what starts it, and cleanup would then kill it. So each native check runs
  under a small re-exec of the runner (`__supervise`) that holds the subreaper role. The child gets
  its own process group. When the child exits or the supervisor is told to stop, it signals the
  group, then every remaining adopted descendant, SIGTERM then SIGKILL, and reaps them.
- **Bazel client.** No subreaper. On a deadline the runner sends one SIGINT to the client it started,
  through its own process handle, waits 10 s, then SIGKILLs that client. One only: Bazel treats
  repeated interrupts as a request to kill its server. This does not depend on custody.
- **Bazel server.** See R7.
- **Bazel lock.** `bazel run` releases the output-base lock before starting the binary, so the runner
  may call Bazel. The first full run witnesses it.
- **macOS.** Process group only. Unverified.

## R6 Capacity

A slot is an exclusive `flock` on `$XDG_CACHE_HOME/bff-template/slots/<n>`, held by the runner for
stages that run Bazel tests. Go opens files close-on-exec, so no child inherits it, and a `flock` ends
with its last open description. A daemon therefore cannot keep a slot. A negative control clears
close-on-exec and must show the failure. Waiting is bounded and ends with exit 10. Pre-commit takes no
slot: it builds nothing past the runner and has a 5 s target. The push worktree has its own lock,
held by the orchestrating parent.

## R7 Daemons

| Server | Owner | Lifetime |
| --- | --- | --- |
| The checkout's server | The developer, and anything else using that checkout | Default idle bound. **The runner never stops it** |
| The push worktree's server | The hooks, while the worktree lock is held | `--max_idle_secs=300` |

The first version of this design would have run `bazel shutdown` and then signaled the PID in
`server.pid.txt`. Independent review rejected it: a runner waiting on a shared server can time out
while another producer owns that server, and neither a PID file, a path nor a capacity lock shows
who it belongs to. So custody has two conditions, and both must hold:

1. **Exclusive output base.** The runner holds the push worktree's lock, and that output base is used
   by nothing else. Only the orchestrator knows this, and it passes it down explicitly. The checkout
   never qualifies.
2. **Exact process.** Read the PID file, open a process handle (a pidfd on Linux, so the PID cannot be
   reused under it), then read `/proc/<pid>/cmdline` and require the exact
   `--output_base=<ours>` argument, and only then signal through that handle. A mismatch, or a
   platform without `/proc`, leaves the process alone and reports it.

A checkout server left after a deadline stays until its idle bound, and the run says so. If it is
wedged, the next client waits, bounded, and reports that. This is the supported behavior, and it
closes the M1 open item by decision, not by killing the process.

A capacity slot bounds how many runs start. It proves nothing about custody.

## R8 Snapshots

- **Pre-commit.** `git diff --cached --name-only -z --diff-filter=ACMRD` selects paths, then
  `git cat-file --batch` writes raw blobs to a scratch directory. Raw bytes avoid filters and
  conversions. Symlinks become files holding their target; gitlinks are skipped. Checks run with the
  scratch directory as their working directory, and the configuration they name comes from the same
  index, because it is what the commit will contain.
- **Pre-push.** Two steps. An orchestrating runner, built from the checkout, takes the worktree lock
  and moves the worktree to the pushed commit. A second runner, built in that worktree by a separate
  output base, judges the commit by that commit's own policy. A dirty checkout can break the
  orchestrator, which fails loudly, but cannot change a verdict.
- **Bootstrap is not a verdict.** Check verdicts come only from staged blobs. But the runner is
  built from the checkout, so a broken file in its own dependency graph (`quality/**`, the root
  `BUILD.bazel`, module files) can stop it from starting. `stage.sh` therefore builds first with
  `bazel run --script_path` and starts the runner as a second step. A failed first step prints a
  `bootstrap ERROR` saying no check ran, exits 2 and never names a check. An untracked file in an
  ordinary checked package is outside that graph and has no effect. Eliminating the case would need a
  runner built from the index on every commit, which costs far more than the 5 s target for a
  failure that is loud, blocks nothing silently and passes nothing.

## R9 Affected tests

Pre-push runs `bazel test //... --test_tag_filters=-e2e`. Bazel's own test cache decides what reruns,
so there is one invocation, no query and no selection that could be wrong. Cost is a warm analysis.
Bazel exit 4 stays `NOT_APPLICABLE`.

## R10 Adapters

`tools/checks/native.sh` is qualified and has fixtures, so it stays one file and moves to
`quality/adapters/native.sh`, instead of splitting into the five directories T054 listed. Input
discovery moves to Go with its tests (`select.go`), so selectors live in the registry. Checks that
judge a Bazel log reuse `test_evidence.sh`.

Moving the repository-specific policy scripts for a consumer's use is M7 work.

## R11 Checks registered

Existing ids are kept where they name a finer check than the first-slice table (`shellcheck`,
`actionlint`, `zizmor`), and contract ids replace the rest (`format-proto`, `lint-proto`,
`format-starlark`, `tests`). The contract table is updated.

| Contract entry | Decision |
| --- | --- |
| `generated-fresh` | Not registered: nothing generated is committed, so it would judge nothing |
| `bazel-lock` | Enforced by `--lockfile_mode=error` on every Bazel call; witnessed by `toolchain-fixtures` |
| `typecheck-ts` | Enforced by `ts_project` in every build, so it is part of `tests` |
| `complexity`, `duplication` | Reviewed, not adopted. No numeric policy is imported from elsewhere; see the gap in `docs/quality-framework.md` |
| `coverage-floor`, `floors-from-base`, `--shard` | M6 |
| `contract-consumers` | Upstream #28 |

T046 asked for `quality/fixtures/<check-id>/` directories. Existing fixtures are generated at test
time so the repository's own scans never see them. Each check instead names its witness target, and
a test requires a named defect and corrected case for each `GUARANTEE`.

## R12 Budgets

`measure.sh` gains the stage scenarios. Records separate edited input from no-change runs, include
failed attempts and peak memory, and name the isolation profile. A no-change cache hit is never
reported as feedback time. Targets remain targets until measured.
