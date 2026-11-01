#!/usr/bin/env bash
# Witness for vulnerabilities.sh (spec 011 FR-002, T023): the real govulncheck against a real server binary, with a
# planted local database: an advisory on a symbol the binary reaches is a finding named in the diagnostic form,
# one fixed before the binary's Go version is not, one on a package the binary lacks is not, and a database or
# a wrapper that cannot be read earns no pass.
#
#   vulnerabilities_test.sh <vulnerabilities.sh> <govulncheck> <binary>
set -euo pipefail
script="$PWD/$1"
govulncheck="$PWD/$2"
binary="$PWD/$3"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

# A wrapper standing for the host's Bazel: it builds nothing and names the binary under test.
cat >"$work/bazel" <<WRAPPER
#!/usr/bin/env bash
if [[ "\$1" == cquery ]]; then echo "$binary"; fi
exit 0
WRAPPER
chmod +x "$work/bazel"

# A local database in the layout govulncheck reads: one advisory, written for each case.
database() { # <dir> <module> <package path> <symbol> [fixed version]
  local dir="$1" module="$2" path="$3" symbol="$4" fixed="${5:-}" stamp="2026-11-01T00:00:00Z" events
  mkdir -p "$dir/index" "$dir/ID"
  events='{"introduced":"0"}'
  if [[ -n "$fixed" ]]; then events="$events,{\"fixed\":\"$fixed\"}"; fi
  printf '{"modified":"%s"}\n' "$stamp" >"$dir/index/db.json"
  printf '[{"path":"%s","vulns":[{"id":"GO-TEST-0001","modified":"%s"}]}]\n' "$module" "$stamp" >"$dir/index/modules.json"
  printf '[{"id":"GO-TEST-0001","modified":"%s","aliases":[]}]\n' "$stamp" >"$dir/index/vulns.json"
  printf '{"schema_version":"1.3.1","id":"GO-TEST-0001","modified":"%s","published":"%s","summary":"Planted advisory for the witness","details":"planted","database_specific":{"url":"https://advisories.example.invalid/GO-TEST-0001"},"affected":[{"package":{"name":"%s","ecosystem":"Go"},"ranges":[{"type":"SEMVER","events":[%s]}],"ecosystem_specific":{"imports":[{"path":"%s","symbols":["%s"]}]}}]}\n' "$stamp" "$stamp" "$module" "$events" "$path" "$symbol" >"$dir/ID/GO-TEST-0001.json"
}

run() { rc=0; bash "$script" "$1" weakened "$2" "$3" //bff/cmd/bff:bff >"$work/out" 2>&1 || rc=$?; }

database "$work/reachable" stdlib crypto/x509 Certificate.Verify
run "$work/bazel" "$govulncheck" "file://$work/reachable"
[[ $rc -eq 1 ]] || { fail "a reachable planted advisory: exit $rc, want 1"; cat "$work/out" >&2; }
grep -q '^vulnerabilities //bff/cmd/bff:bff: GO-TEST-0001: Planted advisory for the witness$' "$work/out" || { fail "the advisory is not named in the diagnostic form"; cat "$work/out" >&2; }
grep -q '^measurement: vulnerabilities //bff/cmd/bff:bff: 1 reachable against file://.* updated 2026-11-01' "$work/out" || fail "the measurement is missing or wrong: $(grep measurement "$work/out")"

database "$work/fixed" stdlib crypto/x509 Certificate.Verify 1.0.0
run "$work/bazel" "$govulncheck" "file://$work/fixed"
[[ $rc -eq 0 ]] || { fail "an advisory fixed before the binary's Go version: exit $rc"; cat "$work/out" >&2; }
grep -q '^measurement: vulnerabilities //bff/cmd/bff:bff: 0 reachable' "$work/out" || fail "the clean measurement is wrong"

database "$work/absent" example.invalid/absent example.invalid/absent/pkg Absent
run "$work/bazel" "$govulncheck" "file://$work/absent"
[[ $rc -eq 0 ]] || { fail "an advisory on a module the binary lacks: exit $rc"; cat "$work/out" >&2; }

run "$work/bazel" "$govulncheck" "file://$work/missing"
[[ $rc -eq 11 ]] || fail "a database that cannot be read: exit $rc, want 11"
run "$work/absent-wrapper" "$govulncheck" "file://$work/reachable"
[[ $rc -eq 11 ]] || fail "no wrapper: exit $rc, want 11"
run "$work/bazel" "$work/absent-tool" "file://$work/reachable"
[[ $rc -eq 11 ]] || fail "no govulncheck: exit $rc, want 11"
rc=0; bash "$script" "$work/bazel" weakened "$govulncheck" "ftp://x" //bff/cmd/bff:bff >"$work/out" 2>&1 || rc=$?
[[ $rc -eq 11 ]] || fail "a database that is not https or file: exit $rc, want 11"
printf '#!/usr/bin/env bash\nexit 1\n' >"$work/broken"; chmod +x "$work/broken"
run "$work/broken" "$govulncheck" "file://$work/reachable"
[[ $rc -eq 11 ]] || fail "a binary that does not build: exit $rc, want 11"

[[ $failures -eq 0 ]] && echo "ok   vulnerabilities"
exit "$failures"
