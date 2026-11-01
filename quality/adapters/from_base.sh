#!/usr/bin/env bash
# floors-from-base and complexity-from-base (spec 007 FR-011, spec 011 R9): builds the named evaluator from a
# detached worktree of the policy base, with the base's own Bazel wrapper and configuration, and runs it against
# the tested tree. Nothing in the tested tree builds or configures it.
#
#   from_base.sh <resolved base SHA> <tested root> <full|weakened> <contract path> <Bazel wrapper> [floors|complexity]
#
# The contract and wrapper paths are the host's. The evaluator comes from the module the base pins: the coverage
# floors evaluator, or the complexity budget comparator. A base that predates the comparator holds nothing to
# compare with; that is said, and nothing is judged, until the base named is at or after the comparator's anchor.
#
# Exit 0 passes, 1 is a finding, and 11 means nothing could be judged. The build is one fixed target
# in a private output base. It runs no test and is not a check invocation. The runner's supervisor
# owns this process tree, so that private server ends with this check.
set -euo pipefail
base="$1"
root="$2"
isolation="$3"
contract="$4"
wrapper="$5"
evaluator="${6:-floors}"
wait_seconds=120
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE
case "$evaluator" in
  floors)
    id=floors-from-base
    target=@bff_template_quality//checks/coverage:floors
    binary=bazel-bin/external/bff_template_quality+/checks/coverage/floors_/floors
    package=quality/checks/coverage
    default_contract=quality/coverage_floors.json
    option=--contract
    ;;
  complexity)
    id=complexity-from-base
    target=@bff_template_quality//checks/complexity:complexity_bin
    binary=bazel-bin/external/bff_template_quality+/checks/complexity/complexity_bin_/complexity_bin
    package=quality/checks/complexity
    default_contract=quality/complexity_budgets.json
    option=--budgets
    ;;
  *)
    echo "from_base: unknown evaluator '$evaluator'; floors or complexity" >&2
    exit 11
    ;;
esac
refuse() { echo "$id: $1" >&2; exit 11; }

common="$(git -C "$root" rev-parse --path-format=absolute --git-common-dir)"
state="$common/bff-template"
mkdir -p "$state"
if [[ -z "${FROM_BASE_LOCKED:-}" ]]; then
  command -v flock >/dev/null 2>&1 || refuse "'flock' not found; install util-linux"
  rc=0
  # flock keeps the lock itself, and --close keeps it from everything below, so no daemon can hold it.
  FROM_BASE_LOCKED=1 flock --close --wait "$wait_seconds" --conflict-exit-code 75 "$state/base.lock" \
    bash "${BASH_SOURCE[0]}" "$@" || rc=$?
  if [[ $rc -eq 75 ]]; then refuse "another run holds the base worktree"; fi
  exit "$rc"
fi

worktree="$state/base-worktree"
sum="$(printf '%s' "$common" | sha256sum | cut -c1-16)"
output_base="${XDG_CACHE_HOME:-$HOME/.cache}/bff-template/base/$sum"
log="$state/base-bootstrap.log"

git -C "$root" worktree prune
if [[ -e "$worktree/.git" ]]; then
  git -C "$worktree" checkout --quiet --detach --force "$base"
  git -C "$worktree" clean -ffdxq
else
  git -C "$root" worktree add --quiet --detach "$worktree" "$base"
fi

if [[ "$evaluator" == complexity && ! -d "$worktree/$package" ]]; then
  # The base predates the comparator's anchor. Nothing can compare the budgets, and the receipt says so.
  echo "$id: the base $base holds no complexity comparator; compared nothing. A base at or after the comparator's anchor compares the budgets"
  exit 0
fi

flags=(--noshow_progress)
if [[ "$isolation" == weakened ]]; then flags+=(--config=weak-isolation); fi
[[ -x "$worktree/$wrapper" ]] || refuse "the base has no Bazel wrapper at $wrapper"
if ! (cd "$worktree" && "$wrapper" "--output_base=$output_base" --max_idle_secs=60 build "${flags[@]}" \
  "$target") </dev/null >"$log" 2>&1; then
  tail -n 20 "$log" >&2
  refuse "the base's evaluator did not build"
fi
evaluator_bin="$worktree/$binary"
[[ -x "$evaluator_bin" ]] || refuse "the base's evaluator did not build: $evaluator_bin is missing"

# An evaluator older than the option knows this one path, so that path is left to its default.
named=()
if [[ "$contract" != "$default_contract" ]]; then named=("$option=$contract"); fi
rc=0
"$evaluator_bin" compare "--base-root=$worktree" "--root=$root" "${named[@]}" </dev/null || rc=$?
case "$rc" in
  0 | 1) exit "$rc" ;;
  *) refuse "the base's evaluator could not judge this tree (exit $rc)" ;;
esac
