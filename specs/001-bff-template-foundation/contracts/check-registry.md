# Contract: Canonical Check Registry and Runner

The registry is the only place a quality rule is defined (constitution IV). Schema:
`quality/proto/registry.proto`; instance: `quality/registry.textproto`. The instance is authoritative
for ids, stages, class and witness. [Classification](../../../docs/quality/classification.md) explains
the classes and lists the rules deliberately left out. M5's reasons are in
[spec 006 research R11](../../006-quality-hooks/research.md).

## Checks

| Group | ids | Stages |
| --- | --- | --- |
| Content | `secrets`, `suppression-ban`, `typos`, `format-go`, `format-ts`, `format-proto`, `lint-proto`, `format-starlark`, `shellcheck`, `actionlint`, `zizmor` | pre-commit, full |
| Pushed commit | `tests`, `dependency-rules`, `proto-breaking` | pre-push, full |
| Full only | `e2e-browser`, `test-inventory`, `sandbox-evidence`, `workspace-policy`, `implicit-config`, `bazelrc-policy`, `toolchain-fixtures`, `hook-e2e`, `speckit-provenance` | full |

Later milestones add `coverage-floor`, `floors-from-base` (M6) and `contract-consumers` (upstream #28).

## Runner interface

```text
bazel run //:quality -- --stage=<pre-commit|commit-msg|pre-push|full>
                        [--isolation=<full|weakened>] [--offline] [--receipt=<path>]
                        [--rerun=<command>] [--jobs=<n>] [--slots=<n>] [--deadline=<duration>]
                        [--from=<rev>] [--to=<rev>] [--inner] [--bazel-startup=<option>] [--shard=<leg>]
```

`--shard=<leg>` runs one leg of `full`, and only with `--stage=full`. Each check is owned by one leg in
the registry, or is a postcondition that runs after the Bazel invocation of every leg that has one
(spec 007 R2). `required_tests.txt` names each required test's leg.

`quality/` is its own module, so the entry is the root target `//:quality`, which embeds
`@bff_template_quality//runner/cmd` and passes the repository's pinned tools as data. The runner
derives its input set itself (see hook-stages.md). It never receives a filename list from prek, and
`tools/hooks/stage.sh` refuses one.

**Exit codes.** The runner's exit codes are deliberately disjoint from Bazel's:

| Code | Meaning |
| --- | --- |
| `0` | every check is non-blocking |
| `1` | at least one check `FAILED` |
| `2` | usage or configuration error, or a `bootstrap ERROR` |
| `10` | `TIMED_OUT` or `CANCELLED` |
| `11` | baseline or evidence unavailable, or a receipt that cannot name what was tested (fail closed) |
| `12` | a `GUARANTEE` check had zero inputs at `full` without `allow_empty` |

Bazel exit codes are translated explicitly:

| Bazel code | Runner result |
| --- | --- |
| `3` (tests failed) | `FAILED` |
| `4` (no tests) on an affected set | `NOT_APPLICABLE` |
| `8` (interrupted) | `CANCELLED` |
| anything else non-zero | `ERROR`, with Bazel's message |

A result that contradicts its own exit code is an `ERROR`, never a pass.

**Diagnostic line**:

```text
<check-id> <path>:<line>:<col>: <message> [code] (rerun: <command>)
```

At most 50 lines are printed per check, followed by `… N more (see <file>)`. That file exists: it is
a private retained log (below), so the omitted lines can be retrieved.

**Execution**:

- All `bazel`-kind checks in a stage run as **one** Bazel invocation. Targets of checks outside the
  stage are subtracted, with no second list.
- `native` checks run in a pool of `--jobs` workers (default `min(4, ncpu/2)`). `exclusive` checks run
  alone, and `after_bazel` checks read that invocation's log.
- stdin is closed for every child.
- Each native check runs under its own supervisor (`__supervise`, a re-exec of the runner) that holds
  a Linux child subreaper. It reaps adopted orphans while the check runs and removes what remains
  at the end.
- On a deadline, the runner sends one SIGINT to the Bazel client it started, and SIGKILL after 10 s.
  It never runs `bazel shutdown` and never signals a checkout's server, which another producer may
  own. It stops a server only under the exclusive push-worktree lock, through a process handle, after
  requiring the exact `--output_base=` argument (spec 006 research R7).
- Heavy stages take a capacity slot, a `flock` held by the parent and closed on exec, so no daemon can
  keep it. A slot proves nothing about ownership of a server.

**Receipts and logs**:

- `<receipt>.lock` is held for the whole run, so one producer owns a receipt path.
- `tools/verify.sh` removes a stale receipt before the runner exists, so that a failed startup leaves no
  earlier result. It takes the same `flock` on the same path for that one removal, and holds nothing
  afterward, so no Bazel process can inherit it. A holder that outlasts 30 s is refused with exit 10,
  and the receipt stays. Without `flock` the wrapper stops with a `bootstrap ERROR`, exit 2, and removes
  nothing. Its overall deadline covers this wait.
- The receipt is written to a unique temporary file and renamed.
- Final bookkeeping shares a 30 s bound, plus up to 9 s to stop a helper. Once the bound has passed,
  no further helper starts. An identity it cannot establish goes in `problems` and denies `PASSED`.
- A failed check's output, up to 1 MiB, is kept mode `0600` in a marked per-run directory beside the
  receipt. The next run on that receipt trims those directories under the lock, so that at most 5
  remain, and removes only marked ones.

**Side effects**: the runner never writes to the index or the working tree.
