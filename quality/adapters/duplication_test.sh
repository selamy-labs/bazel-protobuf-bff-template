#!/usr/bin/env bash
# Witness for duplication.sh (spec 011 FR-002, FR-006): the real dupl, a planted clone pair detected and named,
# the restored control passing, the exclusions holding, and a setup failure earning no pass.
#
#   duplication_test.sh <duplication.sh> <dupl>
set -euo pipefail
script="$PWD/$1"
dupl="$PWD/$2"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

# A function with well over a hundred AST nodes, written twice under different names.
body() { # <package> <function name>
  printf 'package %s\n\nfunc %s(input int) int {\n\tx := input\n' "$1" "$2"
  for i in $(seq 1 12); do
    printf '\tx = x*%d + %d\n\tif x > %d {\n\t\tx -= %d\n\t}\n' "$i" "$((i * 7))" "$((i * 1000))" "$((i * 3))"
  done
  printf '\treturn x\n}\n'
}
repo="$work/repo"
mkdir -p "$repo/a" "$repo/b" "$repo/testdata/c" "$repo/gen"
body a First >"$repo/a/a.go"
body b Second >"$repo/b/b.go"
body c Third >"$repo/testdata/c/c.go"
body a First >"$repo/a/a_test.go"
body gen Generated >"$repo/gen/gen.pb.go"
printf 'package other\n\nfunc Other() int { return 1 }\n' >"$repo/other.go"
printf 'a/a.go\0b/b.go\0testdata/c/c.go\0a/a_test.go\0gen/gen.pb.go\0other.go\0README.md\0' >"$work/all"
printf 'a/a.go\0other.go\0' >"$work/one"
printf 'a/a_test.go\0gen/gen.pb.go\0testdata/c/c.go\0' >"$work/excluded"
: >"$work/none"

run() { rc=0; bash "$script" "$1" "$repo" "$2" "$3" >"$work/out" 2>&1 || rc=$?; }

run "$dupl" "$work/all" 100
[[ $rc -eq 1 ]] || { fail "planted pair: exit $rc, want 1"; cat "$work/out" >&2; }
grep -qE '^duplication a/a\.go:[0-9]+:1: duplicate of b/b\.go:[0-9]+-[0-9]+ \(threshold 100 nodes\)$' "$work/out" || { fail "the pair is not named in the diagnostic form"; cat "$work/out" >&2; }
grep -q '^measurement: duplication 3 files compared at threshold 100; [1-9][0-9]* candidate clone lines$' "$work/out" || fail "the measurement is missing or wrong: $(grep measurement "$work/out")"
grep -q 'testdata/c\|a_test\|gen\.pb' "$work/out" && fail "an excluded file was compared"

run "$dupl" "$work/one" 100
[[ $rc -eq 0 ]] || { fail "restored control: exit $rc"; cat "$work/out" >&2; }
grep -q '^measurement: duplication 2 files compared at threshold 100; 0 candidate clone lines$' "$work/out" || fail "the clean measurement is wrong: $(grep measurement "$work/out")"

run "$dupl" "$work/excluded" 100
[[ $rc -eq 0 ]] || fail "only excluded files: exit $rc"
grep -q 'nothing to compare' "$work/out" || fail "an excluded-only selection did not say it compared nothing"

run "$dupl" "$work/none" 100
[[ $rc -eq 0 ]] || fail "an empty selection: exit $rc"

# A higher threshold than the planted pair's size finds nothing: the threshold is the policy.
run "$dupl" "$work/all" 100000
[[ $rc -eq 0 ]] || fail "a threshold above the pair's size: exit $rc"

run "$work/absent" "$work/all" 100
[[ $rc -eq 11 ]] || fail "no dupl: exit $rc, want 11"
run "$dupl" "$work/all" zero
[[ $rc -eq 11 ]] || fail "a bad threshold: exit $rc, want 11"
run "$dupl" "$work/missing-list" 100
[[ $rc -eq 11 ]] || fail "a missing list: exit $rc, want 11"

[[ $failures -eq 0 ]] && echo "ok   duplication"
exit "$failures"
