#!/usr/bin/env bash
# Checks query output against allowed dependency directions (specs/004-go-bff FR-002, FR-011).
#
#   tools/checks/dependency_rules.sh <rules.tsv> <directory holding <graph>.txt>
#
# A graph with no output fails: a rule over nothing proves nothing.
set -euo pipefail
rules="$1"
graphs="$2"
status=0
checked=0
while IFS=$'\t' read -r graph forbidden; do
  case "$graph" in \#* | "") continue ;; esac
  file="$graphs/$graph.txt"
  if [[ ! -s "$file" ]]; then
    echo "dependency-rules: no dependency output for graph '$graph' (rerun: tools/verify.sh)"
    status=1
    continue
  fi
  checked=$((checked + 1))
  found="$(grep -E -- "$forbidden" "$file" || true)"
  if [[ -n "$found" ]]; then
    while IFS= read -r label; do
      echo "dependency-rules: $graph must not depend on $label (rerun: tools/verify.sh)"
    done <<<"$found"
    status=1
  fi
done <"$rules"
if [[ $checked -eq 0 ]]; then
  echo "dependency-rules: no rule was checked"
  status=1
fi
exit "$status"
