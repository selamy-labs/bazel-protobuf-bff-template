#!/usr/bin/env bash
# Witness for duplication_ts.sh (spec 011 FR-006): the real jscpd over a planted tree. A clone pair of well over the
# threshold is named in the diagnostic form; the restored control passes; an excluded test file carrying the clone
# is not compared; and each setup failure refuses with 11, never a pass.
#
#   duplication_ts_test.sh <duplication_ts.sh> <jscpd> <yq>
set -euo pipefail
script="$(realpath "$1")"
jscpd="$(realpath "$2")"
yq="$(realpath "$3")"
work="$(mktemp -d)"
trap 'rm -rf "${work:?}"' EXIT
mkdir -p "$work/tree/web/a" "$work/tree/web/b"
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
printf 'export const unrelated = (x) => x + 1;\n' >"$work/tree/web/b/other.mjs"
git -C "$work/tree" init -q
git -C "$work/tree" add .
list() { printf '%s\0' "$@" >"$work/list"; }
run() { rc=0; "$script" "$jscpd" "$yq" "$work/tree" "$work/list" "${1:-50}" >"$work/out" 2>"$work/err" || rc=$?; }

list web/a/first.ts web/b/second.ts web/b/other.mjs
run
[[ $rc -eq 1 ]] || { fail "a planted pair: exit $rc, want 1"; cat "$work/out" "$work/err" >&2; }
grep -qE '^duplication-ts web/(a/first|b/second)\.ts:[0-9]+:1: duplicate of web/(a/first|b/second)\.ts:[0-9]+ \([0-9]+ lines, [0-9]+ tokens\)$' "$work/out" \
  || { fail "the pair is not named in the diagnostic form"; cat "$work/out" >&2; }
grep -qE '^measurement: duplication-ts 2 files compared at 50 tokens; [1-9][0-9]* candidate clones \(3 selected; 0 untracked excluded\)$' "$work/out" || fail "no measurement with the candidates"

# The restored control: the clone is gone, and the measurement says so.
printf 'export const alone = (x: number): number => x - 1;\n' >"$work/tree/web/b/second.ts"
run
[[ $rc -eq 0 ]] || { fail "the control: exit $rc"; cat "$work/out" "$work/err" >&2; }
grep -qxF 'measurement: duplication-ts 1 files compared at 50 tokens; 0 candidate clones (3 selected; 0 untracked excluded)' "$work/out" || fail "no clean measurement"

# A test file carrying the clone is not a candidate; neither is generated protocol code or a fixture.
block third >"$work/tree/web/b/second.test.ts"
block fourth >"$work/tree/web/b/notes_pb.ts"
mkdir -p "$work/tree/web/testdata"
block fifth >"$work/tree/web/testdata/planted.ts"
git -C "$work/tree" add web/b/second.test.ts web/b/notes_pb.ts web/testdata/planted.ts
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

# TSX is a separate jscpd format; both sources must be compared.
cp "$work/tree/web/a/first.ts" "$work/tree/web/a/view.tsx"
cp "$work/tree/web/a/first.ts" "$work/tree/web/b/view.tsx"
git -C "$work/tree" add web/a/view.tsx web/b/view.tsx
list web/a/view.tsx web/b/view.tsx
run
[[ $rc -eq 1 ]] || fail "TSX clones escaped: exit $rc"

# Repository-local policy cannot suppress the selected clone pair.
printf '{"ignore":["**"],"format":["python"]}\n' >"$work/tree/.jscpd.json"
printf '*\n' >"$work/tree/.gitignore"
run
[[ $rc -eq 1 ]] || fail "ambient config/ignore suppressed clones: exit $rc"
rm "$work/tree/.jscpd.json" "$work/tree/.gitignore"

# Untracked clones are excluded; tracked absent files and malformed lists refuse.
cp "$work/tree/web/a/first.ts" "$work/tree/web/a/untracked.ts"
list web/a/first.ts web/a/untracked.ts
run
if [[ $rc -ne 0 ]] || ! grep -qF '1 untracked excluded' "$work/out"; then fail "untracked source compared"; fi
list web/a/view.tsx
rm "$work/tree/web/a/view.tsx"
run
[[ $rc -eq 11 ]] || fail "missing tracked source accepted: exit $rc"
printf 'web/a/first.ts' >"$work/list"
run
[[ $rc -eq 11 ]] || fail "unterminated list accepted: exit $rc"
list ../outside.ts
run
[[ $rc -eq 11 ]] || fail "traversal accepted: exit $rc"
list web/a/first.ts
mv "$work/tree/web/a/first.ts" "$work/original.ts"
ln -s "$work/original.ts" "$work/tree/web/a/first.ts"
run
[[ $rc -eq 11 ]] || fail "symbolic source accepted: exit $rc"
rm "$work/tree/web/a/first.ts"
mv "$work/original.ts" "$work/tree/web/a/first.ts"

# Report corruption and a failed tool must never look like zero clones.
cat >"$work/fake" <<'STUB'
#!/usr/bin/env bash
set -eu
while [[ $# -gt 0 ]]; do
  if [[ "$1" == --output ]]; then mkdir -p "$2"; printf '%s' "$REPORT" >"$2/jscpd-report.json"; shift; fi
  shift
done
[[ ${WARNING:-} == '' ]] || echo "$WARNING" >&2
exit "${STATUS:-0}"
STUB
chmod +x "$work/fake"
actual="$jscpd"
jscpd="$work/fake"
invalid_reports=('{}' '' '{"duplicates":null}' '{"duplicates":[],"statistics":{"total":{"sources":0,"clones":0}}}' '{"duplicates":[],"statistics":{"total":{"sources":1,"clones":1}}}' '{"duplicates":[{}],"statistics":{"total":{"sources":1,"clones":1}}}')
for payload in "${invalid_reports[@]}"; do
  REPORT="$payload" run
  [[ $rc -eq 11 ]] || fail "corrupt report accepted: $payload (exit $rc)"
done
export REPORT='{"duplicates":[],"statistics":{"total":{"sources":1,"clones":0}}}' STATUS=42
run
[[ $rc -eq 11 ]] || fail "tool exit 42 accepted: exit $rc"
export STATUS=0 WARNING='could not read selected file'
run
[[ $rc -eq 11 ]] || fail "tool warning accepted: exit $rc"
unset REPORT STATUS WARNING
jscpd="$actual"
list web/b/second.ts web/b/other.mjs
run
[[ $rc -eq 11 ]] || fail "zero analyzed sources reported clean: exit $rc"

if [[ $failures -ne 0 ]]; then
  exit 1
fi
echo "duplication-ts witness passed"
