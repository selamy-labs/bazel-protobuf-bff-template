#!/usr/bin/env bash
# The real graphs pass, and the same rules reject a forbidden edge added to a copy of a real graph.
#
#   tools/checks/dependency_rules_test.sh <script> <rules.tsv> <graph file>...
set -euo pipefail
script="$1"
rules="$2"
shift 2
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

mkdir "$work/real"
for file in "$@"; do cp "$file" "$work/real/$(basename "$file")"; done
chmod -R u+w "$work/real"
"$script" "$rules" "$work/real" || fail "the real graphs broke a rule"
grep -q '^//bff/cmd/bff:bff$' "$work/real/production.txt" || fail "the production graph lacks its binary"
grep -q '^//bff/server:server$' "$work/real/production.txt" || fail "the production graph lacks the server"

reject() { # <name> <graph> <label>
  rm -rf "$work/copy"
  cp -r "$work/real" "$work/copy"
  printf '%s\n' "$3" >>"$work/copy/$2.txt"
  local rc=0
  "$script" "$rules" "$work/copy" >"$work/out" || rc=$?
  if [[ $rc -eq 0 ]] || ! grep -qF "$2 must not depend on $3" "$work/out"; then fail "$1"; fi
}
reject "test identity in production" production //bff/testing/identity:identity
reject "fault injection in production" production //bff/testing/faults:faults
reject "web in production" production //web/app:app
reject "app reaching an adapter" app //bff/adapters/memory:memory
reject "app reaching transport" app //bff/transport:transport
reject "transport reaching an adapter" transport //bff/adapters/memory:memory
reject "ports reaching app" ports //bff/app:app
reject "an adapter reaching app" memory //bff/app:app
reject "the contract reaching the BFF" api //bff/server:server

rm -rf "$work/copy"
cp -r "$work/real" "$work/copy"
: >"$work/copy/app.txt"
if "$script" "$rules" "$work/copy" >/dev/null; then fail "an empty graph passed"; fi

[[ $failures -eq 0 ]] || { echo "$failures witness(es) failed" >&2; exit 1; }
echo "dependency rules passed"
