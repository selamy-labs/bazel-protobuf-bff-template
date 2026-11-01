#!/usr/bin/env bash
# Every GUARANTEE names where its defect and corrected cases live (spec 006 FR-018). A shared fixture
# script must hold both cases by the check's id; any other witness must be a required test or a
# fixture check of its own.
set -euo pipefail
fixtures=tools/checks/native_fixtures_test.sh
required=tools/checks/required_tests.txt
checks=0
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

while read -r id witness; do
  checks=$((checks + 1))
  if [[ "$witness" == "//:native_fixtures_test" ]]; then
    grep -qF "\"$id defect\"" "$fixtures" || fail "$id has no named defect case"
    grep -qF "\"$id corrected\"" "$fixtures" || fail "$id has no named corrected case"
  elif [[ "$witness" != "$id" ]]; then
    grep -qFx "$witness" "$required" || fail "$id: witness $witness is not a required test"
  fi
done < <(awk '/^  id: /{gsub(/"/,""); id=$2} /^  witness: /{gsub(/"/,""); print id, $2}' "$REGISTRY")

[[ $checks -ge 20 ]] || fail "read only $checks checks; the parser is broken, so this proved nothing"
[[ $failures -eq 0 ]] && echo "ok   registry-witness ($checks checks)"
exit "$failures"
