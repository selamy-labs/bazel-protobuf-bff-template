#!/usr/bin/env bash
# Witness for tools/ci/policy_base.sh (spec 007 R6). Real Git, the real script.
set -euo pipefail
script="$PWD/tools/ci/policy_base.sh"
work="$(mktemp -d)"
trap 'rm -rf "${work:?}"' EXIT
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL=a@example.com GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@example.com
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }
cd "$work"
git init -q -b main
git commit -q --allow-empty -m fork
fork="$(git rev-parse HEAD)"
git switch -q -c topic
git commit -q --allow-empty -m topic
git switch -q main
git commit -q --allow-empty -m moved-on
moved="$(git rev-parse HEAD)"
git switch -q topic

expect() { # expect <label> <want output or empty> <want exit> <env...>
  local label="$1" want="$2" code="$3" got rc=0
  shift 3
  got="$(env "$@" bash "$script" 2>"$work/err")" || rc=$?
  [[ $rc -eq $code ]] || fail "$label: exit $rc, want $code"
  [[ "$got" == "$want" ]] || fail "$label: printed '$got', want '$want'"
}
expect "a pull request, base moved on" "sha=$fork" 0 EVENT=pull_request "PR_BASE=$moved"
expect "a merge group" "sha=$moved" 0 EVENT=merge_group "MERGE_BASE=$moved"
expect "a push" "sha=$fork" 0 EVENT=push "PUSH_BEFORE=$fork"
zero="$(printf '0%.0s' {1..40})"
expect "a first push is bootstrap" "" 11 EVENT=push "PUSH_BEFORE=$zero"
grep -qF 'bootstrap' "$work/err" || fail "the first push's refusal named no cause"
expect "an empty push value" "" 11 EVENT=push PUSH_BEFORE=
expect "an empty pull request value" "" 11 EVENT=pull_request PR_BASE=
expect "an unknown pull request base" "" 11 EVENT=pull_request "PR_BASE=$(printf 'a%.0s' {1..40})"
expect "a merge group with no base" "" 11 EVENT=merge_group MERGE_BASE=
expect "a value that is not a commit" "" 11 EVENT=merge_group 'MERGE_BASE=main; rm -rf /'
expect "an unknown event" "" 11 EVENT=schedule
[[ $failures -eq 0 ]] && echo "ok   policy-base"
exit "$failures"
