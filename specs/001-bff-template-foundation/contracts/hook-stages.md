# Contract: Lifecycle Hook Stages (prek adapter)

This contract follows the shared stage and diagnostic contract that is being defined in
selamy-labs/relentless-ci-python#25 and selamy-labs/relentless-ci-typescript#20. Both are open with
no release, so there is no upstream revision to cite yet; `prek.toml` says so, and this repository
adopts upstream's contract when it ships. This repository implements only the Bazel adapter. Its
user-facing behavior and limits are in [docs/quality/hooks.md](../../../docs/quality/hooks.md).

## prek configuration shape (`prek.toml`, M5 T056)

- One local hook per stage.
- `language = "system"`, entry `tools/hooks/stage.sh <stage>`, which scrubs the environment, keeps the
  index Git named, builds `//:quality` and starts it.
- `pass_filenames = false`; the runner derives its own input set (below).
- `require_serial = true`.

| Stage | prek stage | Input set (derived by the runner) | Checks | Budget (provisional, R-11) |
| --- | --- | --- | --- | --- |
| pre-commit | `pre-commit` | `git diff --cached --name-only -z --diff-filter=ACMRD`; content read from the index with `git cat-file`/`git show :<path>` | registry entries with `pre-commit`, which are content checks only | warm p95 ≤ 5 s |
| commit-msg | not installed | the message file | none registered, so no hook is installed; adopting a rule adds both | — |
| pre-push | `pre-push` | the commit at `PRE_COMMIT_TO_REF`, compared with `PRE_COMMIT_FROM_REF`; with no base, as on a first push to an empty remote, the stage escalates to `full` | `pre-push` entries plus affected tests, run in a reusable detached worktree checked out at that commit | warm p95 ≤ 90 s |
| full | `manual` (`prek run --hook-stage manual`) | the whole repository | all entries (`tools/verify.sh`), the same as the union of CI shards | measured |

## Safety rules

1. **Hooks never modify the repository.** Hooks never run `git add`, `git stash` or formatters in
   write mode. prek's own stash of unstaged changes is the only stash. prek takes it at pre-push as
   well as at pre-commit, so a fix that is only unstaged is not in the checkout during a push.
2. **Pre-commit reads only staged blobs.**
   - It reads blob content from the index and never builds from Bazel globs over the working tree.
   - Untracked and unstaged files therefore cannot influence the verdict.
3. **Pre-push checks the pushed commit, not the working tree.**
   - It runs in `$(git rev-parse --git-common-dir)/bff-template/push-worktree`, a detached worktree
     reused across pushes with its own warm output base.
   - prek evaluates only the first pushed ref. A multi-ref push is a documented limit. CI judges pull
     requests, merge groups and `main`; a branch with neither gets no hosted run.
4. **Paths are passed NUL-delimited** end to end. This covers renames, deletions and paths that
   contain spaces or newlines.
5. **Escalation happens at pre-push only.** A change to a policy input, which the registry's
   `escalate` list names, escalates pre-push to `full`. These are the files that shape a verdict:
   the quality module, build and tool configuration, locks, scan configuration, `api/BASELINE`, the
   verifier and its scripts, and frontend lint, build and browser-test configuration. Ordinary code
   and documents do not escalate, and pre-commit never does. A receipt's `policy_digest` covers
   exactly the same paths. Judging whether a policy change is authorized is upstream #26.
6. **Installation** uses `bazel run //tools/hooks:install`, which copies the pinned prek beside the
   Git data, because prek's shim records the path it was installed from, and calls that copy's
   `install` for the configured hook types. If the copy is deleted, the shim takes any `prek` on
   `PATH`; this is a documented limit.
   - prek moves an existing foreign hook aside (`.legacy`) and still runs it; a fixture verifies
     this.
   - Installation honors `core.hooksPath` and worktree-local configuration.
   - Running it twice is a no-op.
7. **Bypass is documented accurately**: `git commit --no-verify`, `git push --no-verify`, and the
   `SKIP`/`PREK_SKIP` variables. `tools/verify.sh` judges the result regardless, and CI runs it once
   CI exists.
8. **Bootstrap is not a verdict.** The runner is built from the checkout, so a file that breaks its own
   dependency graph stops the hook. That prints a `bootstrap ERROR`, exits 2 and names no check. An
   untracked file in an ordinary package is never read.
9. **A server is stopped only with custody.** The checkout's server is shared and is never stopped. The
   push worktree's server is stopped after a deadline, under the worktree's exclusive lock and after
   its exact identity is proved (spec 006 research R7).

## Fixtures (acceptance, T043)

Each fixture runs in a temporary repository:

| Area | Fixtures |
| --- | --- |
| Commit failures | a bad staged secret; a banned suppression; a malformed proto |
| Configuration | a malformed `prek.toml`; a malformed registry |
| Untracked and unstaged content | partial staging with an unrelated unstaged edit (byte-identical afterward); an untracked bad file in the same package (must not affect the verdict) |
| Path handling | a rename; a deletion; a filename with spaces; a filename with a newline |
| Scale | a 500-file batch (whole-project checks run once) |
| Installation | double install; a pre-existing foreign hook; install in a worktree; `core.hooksPath`; the installed-from binary removed; the pinned copy removed (the documented limit) |
| Push | a failing push (a test breaks in the pushed commit while the checkout holds the staged fix); the corrected push; an untracked file in the checkout; a policy change; the multi-ref limit |
| Index | `git commit -a`; `git commit <path>` |
| Bootstrap | a broken runner build, an `ERROR` that names no check |
| Bypass | `SKIP`; `PREK_SKIP`; a bypassed commit and push, caught by `full` |

`//tools/hooks:fixtures_test` replaces only Bazel. `hook-e2e` runs the commit, the push, the bootstrap
failure and both deadline cases against a real Bazel.
