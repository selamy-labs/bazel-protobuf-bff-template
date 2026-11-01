#!/usr/bin/env bash
# duplication (spec 011 FR-006): dupl over the selected first-party Go files at an AST node threshold. A candidate
# clone is a finding; it is deleted in a focused change or recorded with its reason, never suppressed here.
# Generated code, test code and fixtures are not first-party candidates, and each exclusion is named below.
#
#   duplication.sh <dupl> <repository root> <list file: NUL-separated repository-relative paths> <threshold>
#
# Exit 0 with no candidate, 1 with candidates, 11 when nothing could be judged. Each candidate line is the runner's
# diagnostic form; the measurement line is kept in the receipt.
set -euo pipefail
dupl="$1"
root="$2"
list="$3"
threshold="$4"
refuse() { echo "duplication: $1" >&2; exit 11; }
[[ -x "$dupl" ]] || refuse "no dupl at $dupl"
[[ -d "$root" ]] || refuse "no repository root at $root"
[[ -f "$list" ]] || refuse "no file list at $list"
[[ "$threshold" =~ ^[0-9]+$ && "$threshold" -gt 0 ]] || refuse "the threshold must be a positive number of AST nodes, not '$threshold'"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
# Excluded: test files (test code is judged by its own rules), generated protocol code (judged by its generator),
# and fixtures (planted by design).
tr '\0' '\n' <"$list" | grep -E '\.go$' | grep -vE '(_test\.go$|\.pb\.go$|\.connect\.go$|(^|/)testdata/)' | LC_ALL=C sort -u >"$work/files" || true
count="$(grep -c '' "$work/files" || true)"
if [[ "$count" -eq 0 ]]; then
  echo "measurement: duplication 0 files compared at threshold $threshold; nothing to compare"
  exit 0
fi
rc=0
(cd "$root" && "$dupl" -files -plumbing -t "$threshold" <"$work/files") >"$work/out" 2>"$work/err" || rc=$?
# Plumbing lines are "file:from-to: duplicate of file:from-to", one per ordered pair of a clone group.
candidates="$(grep -c ': duplicate of ' "$work/out" || true)"
if [[ $rc -ne 0 && "$candidates" -eq 0 ]]; then
  cat "$work/err" >&2
  refuse "dupl exited $rc"
fi
while IFS= read -r line; do
  [[ -n "$line" ]] || continue
  # "a.go:3-20: duplicate of b.go:5-22" becomes "duplication a.go:3:1: duplicate of b.go:5-22 (threshold N nodes)"
  echo "duplication $(sed -E 's/^([^:]+):([0-9]+)-[0-9]+: /\1:\2:1: /' <<<"$line") (threshold $threshold nodes)"
done <"$work/out"
echo "measurement: duplication $count files compared at threshold $threshold; $candidates candidate clone lines"
[[ "$candidates" -eq 0 ]]
