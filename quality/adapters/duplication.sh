#!/usr/bin/env bash
# spec 011 FR-006: selected tracked Go source only; generated/test/fixture files excluded.
# Paths use a NUL list. dupl's line protocol cannot represent newline/colon paths: refuse them.
# Exit 0 = no candidates, 1 = candidates, 11 = tool/input failure (never a clean scan).
set -euo pipefail
refuse() { echo "duplication: $1" >&2; exit 11; }
[[ $# -eq 4 ]] || refuse "expected dupl, repository, NUL list and threshold"
dupl="$1"; root="$2"; list="$3"; threshold="$4"
[[ -x "$dupl" ]] || refuse "no dupl at $dupl"
[[ -d "$root" ]] || refuse "no repository root at $root"
[[ -f "$list" ]] || refuse "no file list at $list"
[[ "$threshold" =~ ^[0-9]+$ && "$threshold" -gt 0 ]] || refuse "threshold must be positive"
[[ $(git -C "$root" rev-parse --show-prefix 2>/dev/null) == "" ]] || refuse "root must name the git top level"
git -C "$root" rev-parse --is-inside-work-tree >/dev/null 2>&1 || refuse "root is not a git worktree"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
: >"$work/files"
untracked=0
while :; do
  path=''
  if ! IFS= read -r -d '' path; then
    [[ -z "$path" ]] || refuse "unterminated NUL path"
    break
  fi
  case "$path" in
    ''|/*|../*|*/../*|*/..|./*|*$'\n'*|*$'\r'*|*:*) refuse "unsupported or non-relative path" ;;
  esac
  case "$path" in
    *_test.go|*.pb.go|*.connect.go|testdata/*|*/testdata/*) continue ;;
    *.go) ;;
    *) continue ;;
  esac
  if ! git --literal-pathspecs -C "$root" ls-files --error-unmatch -- "$path" >/dev/null 2>&1; then
    untracked=$((untracked+1)); continue
  fi
  [[ -f "$root/$path" && ! -L "$root/$path" ]] || refuse "tracked input absent or symbolic: $path"
  # Parent links must not turn a tracked relative path into an out-of-root read.
  parent="${path%/*}"
  while [[ "$parent" != "$path" && "$parent" != . ]]; do
    [[ ! -L "$root/$parent" ]] || refuse "symbolic parent: $path"
    [[ "$parent" == */* ]] || break
    parent="${parent%/*}"
  done
  printf '%s\n' "$path" >>"$work/files"
done <"$list"
LC_ALL=C sort -u "$work/files" -o "$work/files"
count="$(wc -l <"$work/files" | tr -d ' ')"
if [[ "$count" -eq 0 ]]; then
  echo "measurement: duplication 0 files compared at threshold $threshold; nothing to compare ($untracked untracked excluded)"
  exit 0
fi
rc=0
(cd "$root" && "$dupl" -files -plumbing -t "$threshold" <"$work/files") >"$work/out" 2>"$work/err" || rc=$?
[[ $rc -eq 0 ]] || refuse "dupl exited $rc"
# Under `bazel coverage` the Go runtime of a tool built for coverage writes two lines about its own coverage data
# at exit; they are the runtime's, not dupl's, and only they are set aside before stderr is judged.
grep -vE '^(warning: GOCOVERDIR not set, no coverage data emitted|write meta: error: no meta-data available \(binary not built with -cover\?\))$' "$work/err" >"$work/err.dupl" || true
[[ ! -s "$work/err.dupl" ]] || { cat "$work/err.dupl" >&2; refuse "dupl did not parse every input cleanly"; }
candidates=0
while IFS= read -r line; do
  [[ "$line" =~ ^[^:]+:[0-9]+-[0-9]+:\ duplicate\ of\ [^:]+:[0-9]+-[0-9]+$ ]] || refuse "malformed dupl output"
  candidates=$((candidates+1))
  echo "duplication $(sed -E 's/^([^:]+):([0-9]+)-[0-9]+: /\1:\2:1: /' <<<"$line") (threshold $threshold nodes)"
done <"$work/out"
echo "measurement: duplication $count files compared at threshold $threshold; $candidates candidate clone lines"
[[ "$candidates" -eq 0 ]]
