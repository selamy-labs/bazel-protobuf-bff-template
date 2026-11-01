# Lifecycle hooks

The hooks give fast local feedback. They are not the gate: `tools/verify.sh` is the full policy, and
anything a hook misses or a person bypasses is judged there.

## Install

```bash
tools/bootstrap/bazel.sh run //tools/hooks:install
# On a host without namespace sandboxing, an explicit and labeled choice:
tools/bootstrap/bazel.sh run //tools/hooks:install -- --weak-isolation
```

This copies the pinned prek to `$(git rev-parse --git-common-dir)/bff-template/bin/prek` and installs
from there. A path in Bazel's outputs would disappear with `bazel clean`. Installing twice changes
nothing. It works in linked worktrees and honors `core.hooksPath`.

## Stages

| Stage | Judges | Checks |
| --- | --- | --- |
| `pre-commit` | The staged blobs, from the index Git names, so `git commit -a` and `git commit <path>` are judged on what they will commit | Content checks only. No build and no tests |
| `commit-msg` | Nothing | No check is registered, so no hook is installed |
| `pre-push` | The pushed commit, in a private detached worktree with its own output base, under that commit's own policy | `pre-push` entries and tests. A change to a policy input, or an unknown base, escalates to `full` |
| `full` | The whole repository | Every registry entry. Run `tools/verify.sh`, or `prek run --hook-stage manual` |

The registry, `quality/registry.textproto`, says what each stage runs. There is no second list. To
rerun a stage by hand, use the command each failure prints.

## When a hook refuses

A failure names the check, the file and the rerun command:

```text
FAILED    format-go               0.0s
    format-go src/app.go (rerun: tools/hooks/stage.sh pre-commit)
```

`bootstrap ERROR` is different. It means the runner did not build, so no check ran, and it says
nothing about what is staged. The runner is built from the checkout, so a file that breaks its own
dependency graph stops every hook, even when that file is untracked. Exit code 2 marks this case. An
untracked file in an ordinary package is never read.

## Evidence and logs

Each stage writes `$(git rev-parse --git-common-dir)/bff-template/receipts/<stage>.json`. A stage holds
`<receipt>.lock` for its whole run, so two runs cannot share a receipt.

- A check prints at most 50 lines. Its complete output, up to 1 MiB, is kept in a private `0600` file
  in a directory beside the receipt, and the line after the 50th names that file.
- Each run that has output to keep gets its own private `<stage>.logs-*` directory, marked
  `.quality-run-logs`.
- The next run on the same receipt trims them under the receipt lock, so that at most the newest 5
  remain. It removes only marked directories. To remove them all, delete the directories by hand.
- A secret finding is redacted before it reaches a log.

## Bypass

Bypass is real, and these commands really do skip the check:

| Command | Effect |
| --- | --- |
| `git commit --no-verify` | Skips the commit hook |
| `git push --no-verify` | Skips the push hook, and delivers the commit |
| `SKIP=pre-commit git commit`, `SKIP=pre-push git push` | prek skips that hook |
| `PREK_SKIP=pre-commit git commit` | The same, by prek's own name |

`tools/verify.sh` still judges the result, and the fixtures show it catching a bypassed secret. Without
hosted CI, nothing runs that verifier for you. Hosted CI is not active in this repository until an
operator opens it.

## Limits

- **Only the first pushed ref is judged.** prek evaluates one ref of `git push origin main other`. The
  others reach the remote unchecked by the hook.
- **A deleted pin falls back to `PATH`.** prek's shim runs any `prek` on `PATH` if the copy under the
  Git directory is gone, and that is not the pinned tool. Reinstall to restore the pin.
- **A foreign hook needs a shebang.** prek moves an existing hook to `<hook>.legacy` and runs it first.
  One with no shebang fails every commit with `Exec format error`.
- **prek stashes unstaged changes,** at pre-commit and at pre-push, and restores them afterward. That
  stash is the only one. Hooks never run `git add` or a formatter in write mode.
- **A deadline leaves a shared server running.** After a deadline the runner stops its own Bazel client
  and nothing else, because another build may own the checkout's server. That server exits at its
  normal idle bound. Only the private push server is stopped, and only under its exclusive lock.
- **The first push to an empty remote has no base,** so it escalates to `full`.
- **macOS is unverified.** It has process groups only, with no child subreaper.
