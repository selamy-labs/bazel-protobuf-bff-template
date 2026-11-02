#!/usr/bin/env bash
# python-lint (spec 011 FR-017): the pinned ruff over the selected first-party Python files, lint and format,
# with the repository's own configuration and nothing else: a named configuration file turns ruff's discovery of
# nearer ones off, and no cache is written. A finding is fixed in the file; nothing here suppresses.
#
#   python_lint.sh <ruff> <repository root> <config> <list file: NUL-separated repository-relative paths>
#
# Exit 0 clean, 1 with findings, 11 when nothing could be judged. Each finding is the runner's diagnostic form.
set -euo pipefail
ruff="$1"
root="$2"
config="$3"
list="$4"
refuse() { echo "python-lint: $1" >&2; exit 11; }
[[ -x "$ruff" ]] || refuse "no ruff at $ruff"
[[ -d "$root" ]] || refuse "no repository root at $root"
[[ -f "$root/$config" ]] || refuse "no configuration at $config"
[[ -f "$list" ]] || refuse "no file list at $list"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
# Excluded: fixtures, planted by design (the suppression ban's Python directives among them).
tr '\0' '\n' <"$list" | grep -E '\.py$' | grep -vE '(^|/)testdata/' | LC_ALL=C sort -u >"$work/files" || true
count="$(grep -c '' "$work/files" || true)"
if [[ "$count" -eq 0 ]]; then
  echo "measurement: python-lint 0 files; nothing to lint"
  exit 0
fi
mapfile -t files <"$work/files"
export RUFF_NO_CACHE=1
findings=0
# Lint: one line per finding, "path:line:column: CODE message". ruff exits 1 with findings, 2 on an error.
rc=0
(cd "$root" && "$ruff" check --config "$config" --no-cache --quiet --output-format concise "${files[@]}") \
  >"$work/lint" 2>"$work/err" || rc=$?
if [[ $rc -ne 0 && $rc -ne 1 ]]; then
  cat "$work/err" >&2
  refuse "ruff check exited $rc"
fi
while IFS= read -r line; do
  [[ "$line" =~ ^[^:]+:[0-9]+:[0-9]+:\  ]] || continue
  echo "python-lint $line"
  findings=$((findings + 1))
done <"$work/lint"
# Format: a file the formatter would rewrite is a finding at its first line.
rc=0
(cd "$root" && "$ruff" format --config "$config" --no-cache --quiet --check "${files[@]}") \
  >"$work/format" 2>"$work/err" || rc=$?
if [[ $rc -ne 0 && $rc -ne 1 ]]; then
  cat "$work/err" >&2
  refuse "ruff format exited $rc"
fi
while IFS= read -r line; do
  [[ "$line" =~ ^Would\ reformat:\ (.+)$ ]] || continue
  echo "python-lint ${BASH_REMATCH[1]}:1:1: the formatter would rewrite this file (ruff format)"
  findings=$((findings + 1))
done <"$work/format"
echo "measurement: python-lint $count files; $findings findings"
[[ "$findings" -eq 0 ]]
