#!/usr/bin/env bash
# Witness for duplication_ts.sh (spec 011 FR-006): the real jscpd over a planted tree. A clone pair of well over the
# threshold is named in the diagnostic form; the restored control passes; an excluded test file carrying the clone
# is not compared; and each setup failure refuses with 11, never a pass.
#
#   duplication_ts_test.sh <duplication_ts.sh> <jscpd> <yq>
set -euo pipefail
script="$PWD/$1"
jscpd="$PWD/$2"
yq="$PWD/$3"
work="$(mktemp -d)"
trap 'rm -rf "${work:?}"' EXIT
export HOME="$work/home"
mkdir -p "$HOME" "$work/tree/web/a" "$work/tree/web/b"
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

# A function of well over fifty tokens, planted twice under different names.
block() {
  cat <<EOF
export function $1(items: number[], limit: number): { kept: number[]; dropped: number } {
  const kept: number[] = [];
  let dropped = 0;
  for (const item of items) {
    if (item > limit) {
      dropped += 1;
      continue;
    }
    if (item < 0) {
      kept.push(-item);
    } else {
      kept.push(item * 2 + 1);
    }
  }
  return { kept, dropped };
}
EOF
}
block first >"$work/tree/web/a/first.ts"
block second >"$work/tree/web/b/second.ts"
printf 'export const unrelated = (x: number): number => x + 1;\n' >"$work/tree/web/b/other.mjs"
list() { printf '%s\0' "$@" >"$work/list"; }
run() { rc=0; "$script" "$jscpd" "$yq" "$work/tree" "$work/list" "${1:-50}" >"$work/out" 2>"$work/err" || rc=$?; }

list web/a/first.ts web/b/second.ts web/b/other.mjs
run
[[ $rc -eq 1 ]] || { fail "a planted pair: exit $rc, want 1"; cat "$work/out" "$work/err" >&2; }
grep -qE '^duplication-ts web/(a/first|b/second)\.ts:[0-9]+:1: duplicate of web/(a/first|b/second)\.ts:[0-9]+ \([0-9]+ lines, [0-9]+ tokens\)$' "$work/out" \
  || { fail "the pair is not named in the diagnostic form"; cat "$work/out" >&2; }
grep -qE '^measurement: duplication-ts 3 files compared at 50 tokens; [1-9][0-9]* candidate clones$' "$work/out" || fail "no measurement with the candidates"

# The restored control: the clone is gone, and the measurement says so.
printf 'export const alone = (x: number): number => x - 1;\n' >"$work/tree/web/b/second.ts"
run
[[ $rc -eq 0 ]] || { fail "the control: exit $rc"; cat "$work/out" "$work/err" >&2; }
grep -qxF 'measurement: duplication-ts 3 files compared at 50 tokens; 0 candidate clones' "$work/out" || fail "no clean measurement"

# A test file carrying the clone is not a candidate; neither is generated protocol code or a fixture.
block third >"$work/tree/web/b/second.test.ts"
block fourth >"$work/tree/web/b/notes_pb.ts"
mkdir -p "$work/tree/web/testdata"
block fifth >"$work/tree/web/testdata/planted.ts"
list web/a/first.ts web/b/second.test.ts web/b/notes_pb.ts web/testdata/planted.ts
run
[[ $rc -eq 0 ]] || fail "excluded files were compared: exit $rc"
grep -qF 'duplication-ts 1 files compared' "$work/out" || fail "the exclusions were not applied"

# Nothing selected is nothing compared, and passes with its reason.
list web/README.md
run
[[ $rc -eq 0 ]] || fail "an empty selection: exit $rc"
grep -qF 'nothing to compare' "$work/out" || fail "an empty selection gave no reason"

# A setup failure is a refusal, never a pass.
list web/a/first.ts
rc=0; "$script" "$work/missing" "$yq" "$work/tree" "$work/list" 50 >"$work/out" 2>"$work/err" || rc=$?
[[ $rc -eq 11 ]] || fail "a missing jscpd: exit $rc, want 11"
run abc
[[ $rc -eq 11 ]] || fail "a threshold that is not a number: exit $rc, want 11"
rc=0; "$script" "$jscpd" "$yq" "$work/tree" "$work/no-list" 50 >"$work/out" 2>"$work/err" || rc=$?
[[ $rc -eq 11 ]] || fail "a missing list: exit $rc, want 11"

if [[ $failures -ne 0 ]]; then
  exit 1
fi
echo "duplication-ts witness passed"
