#!/usr/bin/env bash
# CI parity witness (specs/002-hermetic-skeleton US4): the only policy step is the canonical
# verifier, every job feeds the aggregate gate, and the gate fails on any non-success result.
set -euo pipefail
cd "${TEST_SRCDIR}/_main"
wf=".github/workflows/ci.yml"
failures=0
fail() { echo "ci-parity $wf: $1" >&2; failures=$((failures + 1)); }

runs="$(grep -E '^\s+run: ' "$wf" | sed -E 's/^\s+run: //')"
policy="$(grep -vE '^(sudo sysctl -w kernel\.apparmor_restrict_unprivileged_userns=0|\|)$' <<<"$runs" || true)"
[[ "$policy" == "bazel run //:verify" ]] || fail "policy steps must be exactly 'bazel run //:verify', got: $(tr '\n' ';' <<<"$policy")"

jobs="$(awk '/^jobs:/{j=1;next} j && /^  [a-z][a-z0-9_-]*:$/{sub(/:$/,"");sub(/^  /,"");print}' "$wf")"
needs="$(grep -E '^\s+needs: \[' "$wf" | sed -E 's/.*\[(.*)\].*/\1/' | tr -d ' ' | tr ',' '\n')"
for job in $jobs; do
  [[ "$job" == "gate" ]] && continue
  grep -qx "$job" <<<"$needs" || fail "job '$job' is not in gate.needs"
done
grep -q "if: always()" "$wf" || fail "gate must run with if: always()"
grep -q "persist-credentials: false" "$wf" || fail "checkout must not persist credentials"
if grep -E 'uses: ' "$wf" | grep -vqE '@[0-9a-f]{40}( |$)'; then fail "every action must be pinned by commit SHA"; fi

# Execute the workflow's own gate block (not a copy): any non-success result must fail it.
gate_script="$(awk '/^  gate:/{g=1} g && /run: \|/{r=1;next} r && /^          /{sub(/^          /,"");print;next} r{exit}' "$wf")"
[[ -n "$gate_script" ]] || fail "could not find the gate run block"
for case in "success:0" "success skipped:1" "cancelled:1" "failure success:1"; do
  results="${case%:*}" want="${case##*:}" got=0
  RESULTS="$results" sh -c "$gate_script" 2>/dev/null || got=1
  [[ "$got" == "$want" ]] || fail "gate on '$results' exited $got, want $want"
done

[[ $failures -eq 0 ]] && echo "ok   ci-parity"
exit "$failures"
