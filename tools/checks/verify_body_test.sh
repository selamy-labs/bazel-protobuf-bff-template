#!/usr/bin/env bash
# Witness for the verifier's own control flow (specs/002-hermetic-skeleton T025, T032). The real
# verify_body.sh and verify.sh run against stubs of everything they call. Making each check fail in
# turn proves that the check is wired to its real command, that the failure reaches the exit code
# and the receipt, and that no check is recorded as passed by construction.
set -euo pipefail
real_body="$PWD/tools/checks/verify_body.sh"
real_wrapper="$PWD/tools/verify.sh"
required="$PWD/tools/checks/required_tests.txt"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

tree="$work/tree"
mkdir -p "$tree/tools/checks" "$tree/tools/bootstrap" "$tree/tools/policy" "$tree/tools/speckit" "$work/bin"
cp "$real_body" "$tree/tools/checks/verify_body.sh"
cp "$required" "$tree/tools/checks/required_tests.txt"
cp "$PWD/tools/checks/test_evidence.sh" "$tree/tools/checks/test_evidence.sh"
sed 's/^deadline=[0-9]*$/deadline=3/' "$real_wrapper" >"$tree/tools/verify.sh"

stub() { # stub <path> <check id>: exits 1 when that check is the one selected to fail
  cat >"$1" <<STUB
#!/usr/bin/env bash
[[ "\$(cat "$work/fail" 2>/dev/null)" == "$2" ]] && { echo "stub failure: $2"; exit 1; }
exit 0
STUB
  chmod +x "$1"
}
stub "$tree/tools/policy/workspace_policy.sh" workspace-policy
stub "$tree/tools/policy/config_policy.sh" implicit-config
stub "$tree/tools/policy/bazelrc_policy.sh" bazelrc-policy
stub "$tree/tools/policy/lock_fixture.sh" toolchain-fixtures
stub "$tree/tools/speckit/bootstrap.sh" speckit-provenance
stub "$work/bin/suppression" suppression-ban
cat >"$tree/tools/bootstrap/preflight.sh" <<'STUB'
#!/usr/bin/env bash
echo isolation=full
STUB
cat >"$tree/tools/checks/native.sh" <<STUB
#!/usr/bin/env bash
if [[ "\$1" == "\$(cat "$work/hang" 2>/dev/null)" ]]; then
  sleep 300 &
  echo "\$!" >"$work/hung-pid"
  wait
fi
[[ "\$(cat "$work/fail" 2>/dev/null)" == "\$1" ]] && { echo "stub failure: \$1"; exit 1; }
exit 0
STUB
cat >"$tree/tools/checks/inventory.sh" <<STUB
#!/usr/bin/env bash
[[ "\$(cat "$work/fail" 2>/dev/null)" == inventory ]] && exit 1
for f in all authored shell starlark workflows go; do printf 'x\0' >"\$2/\$f"; done
echo x >"\$2/authored.nl"
echo all=1 >"\$2/counts"
git -C "\$1" rev-parse 'HEAD^{tree}' >"\$2/tested_tree"
echo digest >"\$2/policy_digest"
STUB
cat >"$tree/tools/bootstrap/bazel.sh" <<STUB
#!/usr/bin/env bash
case "\$1" in
  --path) echo "$work/bin/tool" ;;
  run)
    for name in ACTIONLINT BUILDIFIER GITLEAKS SHELLCHECK TYPOS UV YQ ZIZMOR; do echo "\$name=$work/bin/tool"; done
    echo "SUPPRESSION=$work/bin/suppression"
    ;;
  test)
    if [[ "\$(cat "$work/fail" 2>/dev/null)" == bazel-test ]]; then exit 3; fi
    if [[ "\$(cat "$work/fail" 2>/dev/null)" == test-inventory ]]; then sed 1d "$required" | sed 's/\$/ PASSED in 0.1s/'
    else sed 's/\$/ PASSED in 0.1s/' "$required"; fi
    if [[ "\$(cat "$work/fail" 2>/dev/null)" == sandbox-evidence ]]; then echo 'INFO: 3 processes: 1 internal, 2 local.'
    else echo 'INFO: 3 processes: 1 internal, 2 linux-sandbox.'; fi
    ;;
esac
STUB
printf '#!/bin/sh\necho "1.0"\n' >"$work/bin/tool"
chmod +x "$work/bin/tool" "$tree/tools/checks/"*.sh "$tree/tools/bootstrap/"*.sh "$tree/tools/verify.sh"
git -C "$tree" init -q
GIT_CONFIG_GLOBAL=/dev/null git -C "$tree" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -q --allow-empty -m base

run() { HOME="$work" bash "$tree/tools/verify.sh" "$@" </dev/null >"$work/out" 2>&1; }
receipt="$tree/.verify/receipt.json"
field() { grep -oE "\"$1\": *\"?[A-Za-z_]+" "$receipt" | head -1 | sed -E 's/.*[": ]//'; }

: >"$work/fail"
rc=0
run || rc=$?
[[ $rc -eq 0 ]] || { fail "all checks passing: exit $rc"; sed 's/^/  /' "$work/out" >&2; }
[[ "$(field result)" == PASSED ]] || fail "all checks passing: receipt result $(field result)"
[[ "$(field isolation)" == full ]] || fail "default isolation label"
expected=(inventory bazel-test test-inventory sandbox-evidence workspace-policy implicit-config bazelrc-policy
  toolchain-fixtures suppression-ban secrets typos shellcheck buildifier actionlint zizmor speckit-provenance)
for id in "${expected[@]}"; do
  grep -qF "{\"check\":\"$id\",\"result\":\"PASSED\"" "$receipt" || fail "$id: not recorded as PASSED in the control run"
done

for id in "${expected[@]}"; do
  echo "$id" >"$work/fail"
  rc=0
  run || rc=$?
  [[ $rc -eq 1 ]] || fail "$id failing: exit $rc, want 1"
  [[ "$(field result)" == FAILED ]] || fail "$id failing: receipt result $(field result)"
  grep -qF "{\"check\":\"$id\",\"result\":\"FAILED\"" "$receipt" || fail "$id failing: receipt does not record it"
done

: >"$work/fail"
run || fail "control before the stale receipt case"
sed -i 's/^echo isolation=full$/exit 1/' "$tree/tools/bootstrap/preflight.sh"
rc=0
run || rc=$?
[[ $rc -ne 0 ]] || fail "failed preflight passed"
[[ ! -e "$receipt" ]] || fail "a previous PASSED receipt survived a failed preflight"
sed -i 's/^exit 1$/echo isolation=full/' "$tree/tools/bootstrap/preflight.sh"

rc=0
run --offline || rc=$?
[[ $rc -eq 0 ]] || fail "offline run: exit $rc"
[[ "$(field result)" == INCOMPLETE ]] || fail "an offline run must not report PASSED, got $(field result)"
grep -qF '{"check":"speckit-provenance","result":"SKIPPED"' "$receipt" || fail "offline skip was not recorded"

run --weak-isolation || true
grep -qF '{"check":"sandbox-evidence","result":"FAILED"' "$receipt" || fail "a linux-sandbox summary under --weak-isolation must not match"

echo shellcheck >"$work/hang"
: >"$work/fail"
rc=0
run || rc=$?
[[ $rc -eq 124 ]] || fail "hung check: exit $rc, want 124"
grep -qF 'overall deadline' "$work/out" || fail "hung check: no diagnostic"
hung="$(cat "$work/hung-pid" 2>/dev/null || true)"
[[ -n "$hung" ]] || fail "the hang stub did not run"
sleep 1
if [[ -n "$hung" ]] && kill -0 "$hung" 2>/dev/null; then
  kill "$hung"
  fail "a grandchild of a bounded check outlived the overall deadline"
fi

[[ $failures -eq 0 ]] && echo "ok   verify-body"
exit "$failures"
