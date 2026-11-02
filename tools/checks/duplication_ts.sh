#!/usr/bin/env bash
# spec 011 FR-006: compare selected tracked TS/TSX/MJS sources with the pinned jscpd.
# Test, generated and planted fixture files are excluded. Exit 0 = clean, 1 = clones,
# 11 = invalid input/tool/report. A report alone never overrides a failed tool.
set -euo pipefail
refuse() { echo "duplication-ts: $1" >&2; exit 11; }
[[ $# -eq 5 ]] || refuse "expected jscpd, yq, repository, NUL list and threshold"
jscpd="$1"; yq="$2"; root="$3"; list="$4"; tokens="$5"
[[ -x "$jscpd" && -x "$yq" ]] || refuse "jscpd and yq must be executable"
[[ -d "$root" && -f "$list" ]] || refuse "repository and file list must exist"
[[ "$tokens" =~ ^[1-9][0-9]*$ && ${#tokens} -le 9 ]] || refuse "threshold must be a positive integer of at most nine digits"
[[ $(git -C "$root" rev-parse --show-prefix 2>/dev/null) == "" ]] || refuse "root must name the git top level"
git -C "$root" rev-parse --is-inside-work-tree >/dev/null 2>&1 || refuse "root is not a git worktree"
jscpd="$(realpath "$jscpd")"; yq="$(realpath "$yq")"; root="$(realpath "$root")"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir "$work/source"
: >"$work/files"
untracked=0
while :; do
  path=''
  if ! IFS= read -r -d '' path; then
    [[ -z "$path" ]] || refuse "unterminated NUL path"
    break
  fi
  case "$path" in
    ''|/*|../*|*/../*|*/..|./*|*/./*|*//*|*$'\n'*|*$'\r'*|*$'\t'*|*:*) refuse "unsupported or non-relative path" ;;
  esac
  case "$path" in
    *.test.ts|*.test.tsx|*.test.mjs|*.spec.ts|*.spec.tsx|*.spec.mjs|*_pb.ts|testdata/*|*/testdata/*) continue ;;
    *.ts|*.tsx|*.mjs) ;;
    *) continue ;;
  esac
  if ! git --literal-pathspecs -C "$root" ls-files --error-unmatch -- "$path" >/dev/null 2>&1; then
    untracked=$((untracked+1)); continue
  fi
  [[ -f "$root/$path" && ! -L "$root/$path" ]] || refuse "tracked input absent or symbolic: $path"
  parent="${path%/*}"
  while [[ "$parent" != "$path" && "$parent" != . ]]; do
    [[ ! -L "$root/$parent" ]] || refuse "symbolic parent: $path"
    [[ "$parent" == */* ]] || break
    parent="${parent%/*}"
  done
  # jscpd otherwise silently skips large files. Name this limit and refuse instead.
  [[ $(wc -c <"$root/$path") -le 1048576 ]] || refuse "input exceeds the explicit 1 MiB limit: $path"
  printf '%s\n' "$path" >>"$work/files"
done <"$list"
LC_ALL=C sort -u "$work/files" -o "$work/files"
count="$(wc -l <"$work/files" | tr -d ' ')"
if [[ "$count" -eq 0 ]]; then
  echo "measurement: duplication-ts 0 files compared at $tokens tokens; nothing to compare ($untracked untracked excluded)"
  exit 0
fi
# An owned snapshot has exactly these files: no ambient config, .gitignore, baseline,
# symlink, or unselected source. One directory argument prevents xargs batching from
# overwriting reports or missing clones across batches. Explicit config disables discovery.
while IFS= read -r path; do
  mkdir -p -- "$(dirname "$work/source/$path")"
  cp -- "$root/$path" "$work/source/$path" || refuse "could not snapshot $path"
done <"$work/files"
printf '{}\n' >"$work/config.json"
rc=0
(cd "$work/source" && env BAZEL_BINDIR=. JS_BINARY__NO_CD_BINDIR=1 JS_BINARY__SILENT_ON_SUCCESS=1 \
  ${RUNFILES_ROOT:+RUNFILES="$RUNFILES_ROOT"} "$jscpd" --config "$work/config.json" --no-gitignore \
  --min-tokens "$tokens" --min-lines 5 --max-size 1048576 --mode mild --format typescript,tsx,javascript \
  --reporters json --output "$work/report" --absolute --silent --no-colors --fail-on-empty .) >"$work/out" 2>"$work/err" || rc=$?
[[ $rc -eq 0 ]] || { cat "$work/err" >&2; refuse "jscpd exited $rc"; }
# Pinned jscpd announces an explicit config on stderr even with --silent.
# That exact informational line is the only allowed diagnostic.
while IFS= read -r diagnostic || [[ -n "$diagnostic" ]]; do
  [[ "$diagnostic" == "Using config from $work/config.json" ]] || refuse "jscpd diagnostic: $diagnostic"
done <"$work/err"
report="$work/report/jscpd-report.json"
[[ -s "$report" && ! -L "$report" ]] || refuse "jscpd wrote no regular nonempty report"
# Require the pinned JSON report's shape, not merely `null | length == 0`.
"$yq" -e -p json -o yaml '((tag == "!!map") and (.duplicates | tag == "!!seq") and
  (.statistics | tag == "!!map") and (.statistics.total.sources | tag == "!!int") and
  (.statistics.total.clones | tag == "!!int"))' "$report" >"$work/shape" || refuse "invalid report shape"
[[ $(cat "$work/shape") == true ]] || refuse "expected exactly one report"
candidates="$("$yq" -p json -o yaml '.duplicates | length' "$report")"
sources="$("$yq" -p json -o yaml '.statistics.total.sources' "$report")"
clones="$("$yq" -p json -o yaml '.statistics.total.clones' "$report")"
[[ "$candidates" =~ ^[0-9]+$ && "$sources" =~ ^[0-9]+$ && "$clones" == "$candidates" ]] || refuse "invalid report counts"
[[ ${#sources} -le 9 && "$sources" -gt 0 && "$sources" -le "$count" ]] || refuse "impossible analyzed-source count"
"$yq" -e -p json -o yaml '[.duplicates[] | select(
  ((.firstFile.name | tag == "!!str") and (.secondFile.name | tag == "!!str") and
   (.firstFile.startLoc.line | tag == "!!int") and (.secondFile.startLoc.line | tag == "!!int") and
   (.lines | tag == "!!int") and (.tokens | tag == "!!int")) | not)] | length == 0' "$report" >/dev/null \
  || refuse "invalid duplicate record"
"$yq" -p json -o tsv '.duplicates[] | [.firstFile.name, .firstFile.startLoc.line, .secondFile.name, .secondFile.startLoc.line, .lines, .tokens]' "$report" >"$work/clones" \
  || refuse "unreadable duplicate records"
while IFS=$'\t' read -r first start second other lines length; do
  [[ "$first" == "$work/source/"* && "$second" == "$work/source/"* ]] || refuse "clone outside selected snapshot"
  first="${first#"$work/source/"}"; second="${second#"$work/source/"}"
  if ! grep -qxF -- "$first" "$work/files" || ! grep -qxF -- "$second" "$work/files"; then
    refuse "clone names unselected source"
  fi
  for number in "$start" "$other" "$lines" "$length"; do
    [[ "$number" =~ ^[1-9][0-9]*$ && ${#number} -le 9 ]] || refuse "invalid clone location/size"
  done
  [[ "$lines" -ge 5 && "$length" -ge "$tokens" ]] || refuse "clone below declared threshold"
  echo "duplication-ts $first:$start:1: duplicate of $second:$other ($lines lines, $length tokens)"
done <"$work/clones"
echo "measurement: duplication-ts $sources files compared at $tokens tokens; $candidates candidate clones ($count selected; $untracked untracked excluded)"
[[ "$candidates" -eq 0 ]]
