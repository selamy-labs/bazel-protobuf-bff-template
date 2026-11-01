#!/usr/bin/env bash
# Controls for the handoff: a failed archive leaves no eligibility record; a verified archive leaves one that names
# the bundle's sha256 and the exact commit and tree; the record never authorizes cleanup.
set -euo pipefail
S="$PWD/$1"; T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
export GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL=a@example.com GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@example.com
git init -q -b main "$T/r"; (cd "$T/r" && printf 'one\n' >a.txt && git add . && git commit -q -m one)
commit="$(git -C "$T/r" rev-parse HEAD)"; failures=0; mkdir -p "$T/rec"
ok() { echo "ok   $1"; }
fail() { echo "FAIL $1"; failures=$((failures + 1)); }
check() { local name="$1"; shift; if "$@"; then ok "$name"; else fail "$name"; fi; }
records() { find "$T/rec" -name '*.eligibility.json' | sort; }
field() { sed -nE "s/.*\"$1\":\"([^\"]+)\".*/\\1/p" "$2"; }
rc=0; "$S" "$T/r" "$commit" "$T/rec" >"$T/out" 2>&1 || rc=$?
check "a failed archive exits 2" test "$rc" == 2
check "a failed archive leaves no eligibility record" test -z "$(records)"
check "the refusal states that cleanup stays ineligible" grep -q 'cleanup stays ineligible' "$T/out"
sleep 1; rc=0; "$S" "$T/r" refs/heads/main "$T/rec" >"$T/out" 2>&1 || rc=$?
rec="$(records | head -1)"
check "a verified archive exits 0" test "$rc" == 0
check "a verified archive leaves a record" test -n "$rec"
check "the record names the commit" grep -q "\"commit\":\"$commit\"" "$rec"
check "the record names a bundle hash" grep -qE '"bundle_sha256":"[0-9a-f]{64}"' "$rec"
check "the record authorizes nothing" grep -q 'not authorized by this record' "$rec"
check "the recorded hash is the bundle's" test "$(sha256sum "$(field bundle "$rec")" | cut -d' ' -f1)" == "$(field bundle_sha256 "$rec")"
echo "handoff controls: $failures failures"; exit "$failures"
