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
    awk -v want="$witness" '$1 == want {found = 1} END {exit !found}' "$required" ||
      fail "$id: witness $witness is not a required test"
  fi
done < <(awk '/^  id: /{gsub(/"/,""); id=$2} /^  witness: /{gsub(/"/,""); print id, $2}' "$REGISTRY")

# Each required test belongs to one leg with Bazel work (spec 007 FR-002).
mapfile -t legs < <(sed -nE 's/^shards: "([a-z0-9-]+)"$/\1/p' "$REGISTRY")
[[ ${#legs[@]} -ge 2 ]] || fail "read only ${#legs[@]} legs; the parser is broken, so this proved nothing"
mapfile -t bazel_legs < <(awk '/^checks \{/{shard = ""} /^  shard: /{gsub(/"/, ""); shard = $2} /kind: KIND_BAZEL/{bazel = 1} /^\}/{if (bazel) print shard; bazel = 0}' "$REGISTRY" | sort -u)
while read -r label leg extra; do
  [[ -n "$label" && "$label" != \#* ]] || continue
  [[ -z "$extra" && " ${bazel_legs[*]} " == *" ${leg:-none} "* ]] || fail "$label: leg '${leg:-}' is not one leg with Bazel work"
done <"$required"

[[ $checks -ge 20 ]] || fail "read only $checks checks; the parser is broken, so this proved nothing"
[[ $failures -eq 0 ]] && echo "ok   registry-witness ($checks checks)"
exit "$failures"
