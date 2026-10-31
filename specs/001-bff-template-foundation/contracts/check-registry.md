# Contract: Canonical Check Registry and Runner

The registry is the only place a quality rule is defined (constitution IV). Schema:
`quality/proto/registry.proto` (M5, T050); instance: `quality/registry.textproto`.

## Initial checks (first slice)

| id | Canonical tool (pinned) | Stages | Class | Notes |
| --- | --- | --- | --- | --- |
| `secrets` | gitleaks | pre-commit, full | GUARANTEE | staged blobs at pre-commit; full history-free tree scan at full |
| `suppression-ban` | runner built-in pattern set | pre-commit, full | GUARANTEE | bans `//nolint`, `eslint-disable`, `@ts-ignore`, `@ts-expect-error`, `// buf:lint:ignore` outside ledger, coverage-ignore markers; stable codes `S001…` |
| `typos` | typos | pre-commit, full | GUARANTEE | |
| `format-go` | gofmt (from Go SDK) | pre-commit, full | GUARANTEE | check mode only |
| `format-ts` | prettier (pnpm lock) | pre-commit, full | GUARANTEE | check mode only |
| `format-proto` / `lint-proto` | buf | pre-commit, full | GUARANTEE | |
| `format-starlark` | buildifier | pre-commit, full | GUARANTEE | |
| `lint-go` | nogo analyzers (vet + staticcheck) | every stage that builds Go (pre-push, full) | GUARANTEE | runs as part of the Go build, not as a separate step. Not enforced until T091 |
| `lint-ts` / `typecheck-ts` | eslint `--max-warnings 0`, tsc | pre-push, full | GUARANTEE | |
| `proto-breaking` | buf breaking vs immutable base SHA | pre-push, full | GUARANTEE | fails closed without baseline |
| `generated-fresh` | build-graph diff | pre-push, full | GUARANTEE | only for committed generated docs |
| `bazel-lock` | `bazel mod deps --lockfile_mode=error` | pre-push, full | GUARANTEE | |
| `dependency-rules` | Bazel query test | pre-push, full | GUARANTEE | FR-007 directions, testonly fencing |
| `tests` | `bazel test` (affected at pre-push, `//...` at full) | pre-push, full | GUARANTEE | |
| `e2e-browser` | Playwright under bazel test | full | GUARANTEE | |
| `coverage-floor` | bazel coverage + floors vs base | full | GUARANTEE | floors only rise |
| `workflow-lint` | actionlint, shellcheck, zizmor | pre-commit (changed workflow), full | GUARANTEE | |
| `floors-from-base` | evaluator built from the base revision, run on the proposal tree | full (CI `policy` shard) | GUARANTEE | suppression ban and coverage floors judged by base policy (R-09) |
| `contract-consumers` | #28 profile (adopted when released) | full | SIGNAL | until then: e2e-per-RPC guarantee + review checklist |
| `complexity` / `duplication` | gocognit / jscpd | full | SIGNAL first, ratcheted later | reviewed in M5 |

## Runner interface

```text
bazel run //quality:run -- --stage=<pre-commit|commit-msg|pre-push|full>
                           [--shard=<name>] [--base=<sha>] [--jobs=<n>]
                           [--receipt=<path>] [--format=text|json]
```

The runner derives its input set itself (see hook-stages.md). It never receives a filename list
from prek.

**Exit codes.** The runner's exit codes are deliberately disjoint from Bazel's:

| Code | Meaning |
| --- | --- |
| `0` | every check is non-blocking |
| `1` | at least one check `FAILED` |
| `2` | usage or configuration error |
| `10` | `TIMED_OUT` or `CANCELLED` |
| `11` | baseline or evidence unavailable (fail closed) |
| `12` | a `GUARANTEE` check had zero inputs at `full` without `allow_empty` |

Bazel exit codes are translated explicitly:

| Bazel code | Runner result |
| --- | --- |
| `3` (tests failed) | `FAILED` |
| `4` (no tests) on an affected set | `NOT_APPLICABLE` |
| `8` (interrupted) | `CANCELLED` |
| anything else non-zero | `ERROR`, with Bazel's message |

**Diagnostic line**:

```text
<check-id> <path>:<line>:<col>: <message> [code] (rerun: <command>)
```

At most 50 lines are printed per check, followed by `… N more (see <receipt>)`.

**Execution**:

- All `bazel`-kind checks in a stage run as **one** Bazel invocation.
- `native` checks run in a pool of `--jobs` workers (default `min(4, ncpu/2)`).
- stdin is closed for every child.
- Native children run in their own process group under a Linux child subreaper.
- On a deadline, the runner sends SIGINT to the Bazel client (then runs `bazel shutdown` if it does
  not exit within 10 s), kills native process groups, and reaps any reparented orphans.

**Side effects**: the runner never writes to the index or the working tree.
