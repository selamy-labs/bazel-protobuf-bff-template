#!/usr/bin/env bash
# Witness for vulnerabilities.sh (spec 011 FR-002, T023): the real govulncheck against a real server binary, with a
# planted local database: an advisory on a symbol the binary reaches is a finding named in the diagnostic form,
# one fixed before the binary's Go version is not, one on a package the binary lacks is not, and a database or
# a wrapper that cannot be read earns no pass. A host with a go command on its PATH fetches nothing, telemetry
# writes nothing, and a read-only tree left in the check's temporary HOME neither changes the verdict nor stays.
#
#   vulnerabilities_test.sh <vulnerabilities.sh> <govulncheck> <binary>
set -euo pipefail
script="$PWD/$1"
govulncheck="$PWD/$2"
binary="$PWD/$3"
work="$(mktemp -d)"
trap 'chmod -R u+w "$work" 2>/dev/null || true; rm -rf "$work"' EXIT
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

# A hosted runner has a go command on its PATH. `govulncheck -version` runs `go env GOVERSION`, and in this
# repository that go honors go.mod by downloading a toolchain into $HOME, whose module cache Go leaves read-only
# (0555 directories, 0444 files). This go leaves that tree behind whatever it is told, so the trap is witnessed
# even if GOTOOLCHAIN stops the real download. It also records what it was told, and what the telemetry library
# has written by the time govulncheck calls it: that library starts a child that outlives the scan and writes
# while the trap removes the directory, unless its mode file says off. A scan is too quick to lose that race on
# demand, so the witness reads the directory instead of waiting for the race.
mkdir -p "$work/host-go" "$work/tmp" "$work/host-config"
cat >"$work/host-go/go" <<GO
#!/usr/bin/env bash
printf '%s\n' "\${GOTOOLCHAIN:-unset}" >>"$work/go-calls"
telemetry="\${XDG_CONFIG_HOME:-\$HOME/.config}/go/telemetry"
ls -A "\$telemetry" >>"$work/telemetry-seen" 2>/dev/null || echo unreadable >>"$work/telemetry-seen"
cat "\$telemetry/mode" >>"$work/telemetry-mode" 2>/dev/null || echo absent >>"$work/telemetry-mode"
at=@ # split, or the neutrality scan reads Go's cache directory name as an address
cache="\$HOME/go/pkg/mod/golang.org/toolchain\${at}v0.0.1-go1.27.3.linux-amd64"
if [[ ! -d "\$cache" ]]; then
  mkdir -p "\$cache/bin"
  printf 'downloaded\n' >"\$cache/bin/go"
  chmod -R a-w "\$HOME/go/pkg/mod/golang.org"
fi
echo go1.27.3
GO
chmod +x "$work/host-go/go"
hosted() { # <database>: TMPDIR is this witness's, so what the check leaves behind can be seen.
  rm -f "$work/go-calls" "$work/telemetry-seen" "$work/telemetry-mode"
  rc=0; XDG_CONFIG_HOME="$work/host-config" TMPDIR="$work/tmp" PATH="$work/host-go:$PATH" bash "$script" "$work/bazel" weakened "$govulncheck" "$1" //bff/cmd/bff:bff >"$work/out" 2>&1 || rc=$?
}
reached() { [[ -s "$work/go-calls" ]] || fail "$1: the go command was never reached, so nothing was planted"; }
clean() {
  [[ -z "$(ls -A "$work/tmp")" ]] || fail "$1: the temporary directory was left behind"
  [[ -z "$(ls -A "$work/host-config")" ]] || fail "$1: the check wrote into the host's configuration directory"
}

hosted "file://$work/fixed"
[[ $rc -eq 0 ]] || { fail "a clean scan over a read-only tree: exit $rc, want 0"; cat "$work/out" >&2; }
grep -q '^measurement: vulnerabilities //bff/cmd/bff:bff: 0 reachable' "$work/out" || fail "a clean scan over a read-only tree gave no measurement"
reached "a clean scan"
clean "a clean scan"
grep -qvx local "$work/go-calls" && fail "the go command may fetch a toolchain: GOTOOLCHAIN was $(sort -u "$work/go-calls" | tr '\n' ' ')"
grep -qvx off "$work/telemetry-mode" && fail "the telemetry mode is not off: $(sort -u "$work/telemetry-mode" | tr '\n' ' ')"
grep -qvx mode "$work/telemetry-seen" && fail "telemetry is writing: $(sort -u "$work/telemetry-seen" | tr '\n' ' ')"

# The verdict survives the cleanup: a finding stays a finding, and it does not depend on the host's go.
hosted "file://$work/reachable"
[[ $rc -eq 1 ]] || { fail "a finding over a read-only tree: exit $rc, want 1"; cat "$work/out" >&2; }
grep -q '^vulnerabilities //bff/cmd/bff:bff: GO-TEST-0001: Planted advisory for the witness$' "$work/out" || fail "the finding over a read-only tree is not named"
reached "a finding"
clean "a finding"

[[ $failures -eq 0 ]] && echo "ok   vulnerabilities"
exit "$failures"
