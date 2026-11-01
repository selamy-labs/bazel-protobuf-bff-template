#!/usr/bin/env bash
# The production bundle has no test identity and carries the sign-in, and the check can see the test identity
# when it is there (spec 012 FR-016).
#
#   web/app/bundle_test.sh <production dist> <dev dist>
set -euo pipefail
production="$1"
development="$2"
find -L "$production" -name '*.js' | grep -q . || { echo "FAIL no production bundle" >&2; exit 1; }
# The sign-in is the code flow with a proof key: its discovery path and its challenge method name it.
grep -Rq 'openid-configuration' "$production" || { echo "FAIL the production bundle has no sign-in" >&2; exit 1; }
grep -Rq 'code_challenge_method' "$production" || { echo "FAIL the production bundle's sign-in carries no proof key" >&2; exit 1; }
if grep -RIl -e 'test-identity' -e 'Test identity' "$production"; then
  echo "FAIL the production bundle contains test identity" >&2
  exit 1
fi
if grep -RIl -e 'planted-render-error' -e 'planted render error' "$production"; then
  echo "FAIL the production bundle contains the planted render error" >&2
  exit 1
fi
grep -Rq 'Sign-in is not configured' "$production" || { echo "FAIL production is not the bundle tested" >&2; exit 1; }
grep -Rq 'test-identity\.' "$development" || { echo "FAIL the check cannot see test identity" >&2; exit 1; }
grep -Rq 'planted-render-error' "$development" || { echo "FAIL the check cannot see the planted render error" >&2; exit 1; }
echo "bundle checks passed"
