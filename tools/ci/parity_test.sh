#!/usr/bin/env bash
# CI parity witness (specs/002-hermetic-skeleton T024). The real workflow must pass the closed
# policy. Every adversarial mutation must actually change it and be rejected for its named reason.
# The gate's own run block is executed, not a copy of it.
set -euo pipefail
yq="$(realpath "$1")"
ciparity="$(realpath "$2")"
registry="$(realpath "$3")"
cd "${TEST_SRCDIR}/_main"
wf=".github/workflows/ci.yml"
policy="tools/ci/policy.json"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

mapfile -d '' workflows < <(find -L .github/workflows -type f -print0)
[[ ${#workflows[@]} -eq 1 ]] || fail "exactly one workflow is expected, found: ${workflows[*]}"

"$yq" -o=json 'explode(.)' "$wf" >"$work/base.json"
"$ciparity" "$work/base.json" "$policy" "$registry" >"$work/base.out" 2>&1 || {
  cat "$work/base.out" >&2
  fail "the real workflow must pass"
}
"$yq" -o=json 'explode(.)' "$wf" | cmp -s - "$work/base.json" || fail "conversion is not stable"

cases=0
while IFS=$'\t' read -r name expr want; do
  [[ -z "$name" || "$name" == \#* ]] && continue
  cases=$((cases + 1))
  if ! "$yq" -o=json "explode(.) | $expr" "$wf" >"$work/m.json" 2>"$work/err"; then
    fail "$name: mutation did not apply: $(head -1 "$work/err")"
    continue
  fi
  if [[ ! -s "$work/m.json" ]] || cmp -s "$work/m.json" "$work/base.json"; then
    fail "$name: mutation changed nothing, so it proves nothing"
    continue
  fi
  rc=0
  "$ciparity" "$work/m.json" "$policy" "$registry" >"$work/out" 2>&1 || rc=$?
  if [[ $rc -ne 1 ]]; then
    fail "$name: exit $rc, want 1 (accepted or unjudged)"
  elif ! grep -qF -- "$want" "$work/out"; then
    fail "$name: rejected, but not for '$want': $(head -1 "$work/out")"
  fi
done <tools/ci/mutations.tsv
[[ $cases -ge 48 ]] || fail "expected at least 48 mutation cases, ran $cases"

: >"$work/empty.json"
rc=0
"$ciparity" "$work/empty.json" "$policy" "$registry" >/dev/null 2>&1 || rc=$?
[[ $rc -eq 2 ]] || fail "empty input must be unjudged (exit 2), got $rc"

"$yq" '.jobs.gate.steps[0].run' "$wf" >"$work/gate.sh"
[[ -s "$work/gate.sh" ]] || fail "could not extract the gate run block"
# The run block's input is what `needs` reports: the latest result of each leg. So a leg that failed and
# was rerun to success, beside legs from an earlier attempt, is three successes (spec 007 R4). That
# GitHub reports it so is unobserved here: no hosted run exists.
gate() { # gate <results> <want>
  local got=0
  RESULTS="$1" EXPECTED_LEGS=3 sh "$work/gate.sh" 2>/dev/null || got=1
  [[ "$got" == "$2" ]] || fail "gate on '$1' exited $got, want $2"
}
gate "success success success" 0
gate "success failure success" 1
gate "success skipped success" 1
gate "cancelled success success" 1
gate "success success failure" 1
gate "success success" 1
gate "success" 1
gate "" 1
gate "success success success success" 1

[[ $failures -eq 0 ]] && echo "ok   ci-parity ($cases mutations rejected)"
exit "$failures"
