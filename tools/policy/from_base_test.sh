#!/usr/bin/env bash
# Witness for the module's from_base.sh (spec 007 FR-011, FR-013). Real Git, real flock, the real script.
# Only the base's Bazel is a stub, and it is committed at the base, so running the proposal's copy
# shows up as a poisoned result.
#
#   tools/policy/from_base_test.sh <from_base.sh>
set -euo pipefail
script="$PWD/$1"
work="$(mktemp -d)"
trap 'rm -rf "${work:?}"' EXIT
export HOME="$work/home" XDG_CACHE_HOME="$work/cache" PATH=/usr/bin:/bin
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL=a@example.com GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@example.com
mkdir -p "$HOME"
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

repo="$work/repo"
mkdir -p "$repo/tools/bootstrap"
git -C "$repo" init -q -b main
# The stub stands for the base's own Bazel. It records its arguments and where it ran, and builds a
# floors stub whose exit code the test sets.
cat >"$repo/tools/bootstrap/bazel.sh" <<STUB
#!/usr/bin/env bash
{ pwd; printf '%s\n' "\$@"; git rev-parse HEAD; } >"$work/built"
[[ ! -e "$work/build-fails" ]] || { echo "the build broke"; exit 1; }
mkdir -p bazel-bin/external/bff_template_quality+/checks/coverage/floors_
cat >bazel-bin/external/bff_template_quality+/checks/coverage/floors_/floors <<'INNER'
#!/usr/bin/env bash
printf '%s\n' "\$@" >"$work/floors"
exit "\$(cat "$work/floors-exit" 2>/dev/null || echo 0)"
INNER
chmod +x bazel-bin/external/bff_template_quality+/checks/coverage/floors_/floors
STUB
chmod +x "$repo/tools/bootstrap/bazel.sh"
echo base >"$repo/marker"
git -C "$repo" add -A
git -C "$repo" commit -q -m base
base="$(git -C "$repo" rev-parse HEAD)"
printf '#!/bin/sh\ntouch "%s/poisoned"\nexit 0\n' "$work" >"$repo/tools/bootstrap/bazel.sh"
echo 'build --poisoned' >"$repo/.bazelrc"
echo proposal >"$repo/marker"
git -C "$repo" add -A
git -C "$repo" commit -q -m proposal

contract=quality/coverage_floors.json
run() { rc=0; bash "$script" "$base" "$repo" "${1:-full}" "$contract" tools/bootstrap/bazel.sh </dev/null >"$work/out" 2>&1 || rc=$?; }
worktree="$repo/.git/bff-template/base-worktree"

run
[[ $rc -eq 0 ]] || { fail "the control: exit $rc"; cat "$work/out" >&2; }
[[ ! -e "$work/poisoned" ]] || fail "the proposal's Bazel wrapper ran"
[[ "$(sed -n 1p "$work/built")" == "$(cd "$worktree" && pwd -P)" ]] || fail "the build did not run in the base worktree"
[[ "$(tail -n 1 "$work/built")" == "$base" ]] || fail "the build was not at the base"
grep -q -- "^--output_base=$work/cache/bff-template/base/" "$work/built" || fail "no private output base"
grep -qxF 'build' "$work/built" || fail "not a build"
grep -qE '^(test|coverage)$' "$work/built" && fail "the bootstrap ran tests"
grep -qxF '@bff_template_quality//checks/coverage:floors' "$work/built" || fail "not the fixed target"
grep -qxF -- "--base-root=$worktree" "$work/floors" || fail "the evaluator was not given the base's tree"
grep -qxF -- "--root=$repo" "$work/floors" || fail "the evaluator was not given the tested tree"
[[ "$(cat "$worktree/marker")" == base ]] || fail "the worktree is not the base's content"
[[ ! -e "$worktree/.bazelrc" ]] || fail "the proposal's .bazelrc reached the base worktree"
grep -qxF -- '--config=weak-isolation' "$work/built" && fail "full isolation asked for the weak config"

run weakened
[[ $rc -eq 0 ]] || fail "weakened: exit $rc"
grep -qxF -- '--config=weak-isolation' "$work/built" || fail "the weakened label did not reach the build"

touch "$worktree/left-behind"
run
[[ ! -e "$worktree/left-behind" ]] || fail "an earlier run's file survived in the worktree"

echo 1 >"$work/floors-exit"
run
[[ $rc -eq 1 ]] || fail "a finding: exit $rc, want 1"
echo 2 >"$work/floors-exit"
run
[[ $rc -eq 11 ]] || fail "an evaluator that could not judge: exit $rc, want 11"
grep -qF 'could not judge' "$work/out" || fail "no cause named"
rm -f "$work/floors-exit"

touch "$work/build-fails"
run
[[ $rc -eq 11 ]] || fail "a base that does not build: exit $rc, want 11"
grep -qF "the base's evaluator did not build" "$work/out" || fail "no cause named"
grep -qF 'the build broke' "$work/out" || fail "the build's own output was lost"
rm -f "$work/build-fails"

# One holder at a time, within a bound, and the holder's lock is the file's, not an inherited one.
sed 's/^wait_seconds=[0-9]*$/wait_seconds=1/' "$script" >"$work/short.sh"
flock "$repo/.git/bff-template/base.lock" sleep 5 &
holder=$!
for _ in $(seq 1 50); do
  if ! flock -n "$repo/.git/bff-template/base.lock" true; then break; fi
  sleep 0.1
done
rc=0
bash "$work/short.sh" "$base" "$repo" full "$contract" tools/bootstrap/bazel.sh </dev/null >"$work/out" 2>&1 || rc=$?
[[ $rc -eq 11 ]] || fail "a held worktree: exit $rc, want 11"
grep -qF 'another run holds the base worktree' "$work/out" || fail "no cause named"
wait "$holder"

# A host names its own paths, and the base's evaluator is told only when it differs from the default an
# older evaluator knows.
run
grep -qx -- '--contract=quality/coverage_floors.json' "$work/floors" && fail "the default contract was named to an older evaluator"
contract=policy/contract.json
run
[[ $rc -eq 0 ]] || fail "a host's contract: exit $rc"
grep -qx -- '--contract=policy/contract.json' "$work/floors" || fail "the host's contract did not reach the evaluator"
contract=quality/coverage_floors.json
rc=0
bash "$script" "$base" "$repo" full "$contract" tools/none.sh </dev/null >"$work/out" 2>&1 || rc=$?
[[ $rc -eq 11 ]] || fail "a base with no wrapper: exit $rc, want 11"
grep -qF 'no Bazel wrapper at tools/none.sh' "$work/out" || fail "the missing wrapper was not named"

[[ $failures -eq 0 ]] && echo "ok   from-base"
exit "$failures"
