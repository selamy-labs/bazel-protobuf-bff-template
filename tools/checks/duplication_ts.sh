#!/usr/bin/env bash
# duplication-ts (spec 011 FR-006): jscpd over the selected first-party TypeScript and JavaScript files at a token
# threshold. A candidate clone is a finding; it is deleted in a focused change or recorded with its reason, never
# suppressed here. Test code, generated protocol code and fixtures are not first-party candidates, and each
# exclusion is named below. The pinned jscpd is a prebuilt binary the npm package resolves for this platform.
#
#   duplication_ts.sh <jscpd> <yq> <repository root> <list file: NUL-separated repository-relative paths> <min tokens>
#
# Exit 0 with no candidate, 1 with candidates, 11 when nothing could be judged. Each candidate line is the runner's
# diagnostic form; the measurement line is kept in the receipt.
set -euo pipefail
jscpd="$1"
yq="$2"
root="$3"
list="$4"
tokens="$5"
refuse() { echo "duplication-ts: $1" >&2; exit 11; }
[[ -x "$jscpd" ]] || refuse "no jscpd at $jscpd"
[[ -x "$yq" ]] || refuse "no yq at $yq"
[[ -d "$root" ]] || refuse "no repository root at $root"
[[ -f "$list" ]] || refuse "no file list at $list"
[[ "$tokens" =~ ^[0-9]+$ && "$tokens" -gt 0 ]] || refuse "the threshold must be a positive number of tokens, not '$tokens'"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
# Excluded: test files and browser specs (judged by their own rules), generated protocol code (judged by its
# generator), and fixtures (planted by design).
tr '\0' '\n' <"$list" | grep -E '\.(ts|tsx|mjs)$' \
  | grep -vE '(\.test\.(ts|mjs)$|\.spec\.mjs$|_pb\.ts$|(^|/)testdata/)' | LC_ALL=C sort -u >"$work/files" || true
count="$(grep -c '' "$work/files" || true)"
if [[ "$count" -eq 0 ]]; then
  echo "measurement: duplication-ts 0 files compared at $tokens tokens; nothing to compare"
  exit 0
fi
rc=0
# The npm package's launcher is a Bazel js_binary: it wants the build directory named, and it may not change
# directory, as the native adapter runs prettier.
(cd "$root" && xargs -d '\n' env BAZEL_BINDIR=. JS_BINARY__NO_CD_BINDIR=1 JS_BINARY__SILENT_ON_SUCCESS=1 \
  ${RUNFILES_ROOT:+RUNFILES="$RUNFILES_ROOT"} "$jscpd" --min-tokens "$tokens" --min-lines 5 --format typescript,javascript \
  --reporters json --output "$work/report" --absolute --silent <"$work/files") >"$work/out" 2>"$work/err" || rc=$?
report="$work/report/jscpd-report.json"
if [[ ! -f "$report" ]]; then
  cat "$work/out" "$work/err" >&2
  refuse "jscpd exited $rc and wrote no report"
fi
candidates="$("$yq" -p json -o yaml '.duplicates | length' "$report")" || refuse "the report could not be read"
[[ "$candidates" =~ ^[0-9]+$ ]] || refuse "the report names no duplicate count"
# "web/a.ts:3:1: duplicate of web/b.ts:5 (12 lines, 80 tokens)": the first location in the runner's diagnostic form.
[[ "$candidates" -eq 0 ]] || "$yq" -p json -o yaml '.duplicates[] | "duplication-ts " + .firstFile.name + ":" + ((.firstFile.startLoc.line // .firstFile.start) | tostring) + ":1: duplicate of " + .secondFile.name + ":" + ((.secondFile.startLoc.line // .secondFile.start) | tostring) + " (" + (.lines | tostring) + " lines, " + (.tokens | tostring) + " tokens)"' "$report" \
  | sed -E "s#$root/##g"
echo "measurement: duplication-ts $count files compared at $tokens tokens; $candidates candidate clones"
[[ "$candidates" -eq 0 ]]
