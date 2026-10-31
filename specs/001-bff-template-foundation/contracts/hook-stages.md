# Contract: Lifecycle Hook Stages (prek adapter)

This contract follows the shared stage and diagnostic contract that is being defined in
selamy-labs/relentless-ci-python#25 and selamy-labs/relentless-ci-typescript#20. T056 records the
upstream revision it follows. This repository implements only the Bazel adapter.

## prek configuration shape (`prek.toml`, M5 T056)

- One local hook per stage.
- `language = "system"`, invoking the pinned runner through Bazel.
- `pass_filenames = false`; the runner derives its own input set (below).
- `require_serial = true`.

| Stage | prek stage | Input set (derived by the runner) | Checks | Budget (provisional, R-11) |
| --- | --- | --- | --- | --- |
| pre-commit | `pre-commit` | `git diff --cached --name-only -z --diff-filter=ACMRD`; content read from the index with `git cat-file`/`git show :<path>` | registry entries with `pre-commit`, which are content checks only | warm p95 ≤ 5 s |
| commit-msg | `commit-msg` | the message file | none by default; adopted explicitly | — |
| pre-push | `pre-push` | the commit at `PRE_COMMIT_TO_REF`, compared with `PRE_COMMIT_FROM_REF` (or the merge-base with the remote default branch for new refs) | `pre-push` entries plus affected tests, run in a reusable detached worktree checked out at that commit | warm p95 ≤ 90 s |
| full | `manual` (`prek run --hook-stage manual`) | the whole repository | all entries (`tools/verify.sh`), the same as the union of CI shards | measured |

## Safety rules

1. **Hooks never modify the repository.** Hooks never run `git add`, `git stash` or formatters in
   write mode. prek's own stash of unstaged changes is the only stash.
2. **Pre-commit reads only staged blobs.**
   - It reads blob content from the index and never builds from Bazel globs over the working tree.
   - Untracked and unstaged files therefore cannot influence the verdict.
3. **Pre-push checks the pushed commit, not the working tree.**
   - It runs in `$(git rev-parse --git-common-dir)/bff-template/push-worktree`, a detached worktree
     reused across pushes with its own warm output base.
   - prek evaluates only the first pushed ref. A multi-ref push is a documented limit; CI checks
     every ref.
4. **Paths are passed NUL-delimited** end to end. This covers renames, deletions and paths that
   contain spaces or newlines.
5. **Escalation happens at pre-push only.** A change to `quality/**`, `prek.toml`, `.bazelrc`,
   `.bazelversion`, `MODULE.bazel*` or the tool locks escalates pre-push to `full`. Pre-commit never
   escalates.
6. **Installation** uses `bazel run //tools/hooks:install`, which calls the pinned prek's `install`
   for the configured hook types.
   - prek moves an existing foreign hook aside (`.legacy`) and still runs it; a fixture verifies
     this.
   - Installation honors `core.hooksPath` and worktree-local configuration.
   - Running it twice is a no-op.
7. **Bypass is documented accurately**: `git commit --no-verify`, `git push --no-verify`, and the
   `SKIP`/`PREK_SKIP` variables. CI runs `full` regardless.

## Fixtures (acceptance, T043)

Each fixture runs in a temporary repository:

| Area | Fixtures |
| --- | --- |
| Commit failures | a bad staged secret; a banned suppression; a malformed proto |
| Configuration | a malformed `prek.toml`; a malformed registry |
| Untracked and unstaged content | partial staging with an unrelated unstaged edit (byte-identical afterward); an untracked bad file in the same package (must not affect the verdict) |
| Path handling | a rename; a deletion; a filename with spaces |
| Scale | a 500-file batch (whole-project checks run once) |
| Installation | double install; a pre-existing foreign hook; install in a worktree |
| Push | a failing push (a test breaks in the pushed commit while the working tree is clean); the corrected push |
| Bypass | a bypassed commit and push, caught by `full` |
