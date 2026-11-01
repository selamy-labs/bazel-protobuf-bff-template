#!/usr/bin/env bash
# Witness for the verifier's control flow (spec 002 T025, T032; spec 006 T011). The real wrapper, the
# real runner, the real registry and the real evidence judge run against stubs of the tools. Making
# each registered check fail in turn proves that it is wired, that the failure reaches the exit code
# and the receipt, and that no check is recorded as passed by construction. The ids come from the
# registry, so this test holds no list of its own.
#
#   tools/checks/verify_body_test.sh <runner>
set -euo pipefail
runner="$PWD/$1"
registry="$PWD/${REGISTRY:?}"
required="$PWD/tools/checks/required_tests.txt"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
labels="$work/labels"
awk '!/^#/ {print $1}' "$required" >"$labels"
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

tree="$work/tree"
mkdir -p "$tree/tools/checks" "$tree/tools/bootstrap" "$tree/tools/policy" "$tree/tools/hooks" "$tree/tools/speckit" "$work/bin" \
  "$tree/api" "$tree/.github/workflows"
cp tools/checks/verify_body.sh tools/checks/required_tests.txt tools/checks/test_evidence.sh "$tree/tools/checks/"
sed 's/^deadline=[0-9]*$/deadline=20/' tools/verify.sh >"$tree/tools/verify.sh"
sed -i 's/--kill-after=60/--kill-after=1/; s/^receipt_wait=[0-9]*$/receipt_wait=3/' "$tree/tools/checks/verify_body.sh"

stub() { # stub <path> <check id>: exits 1 when that check is the one selected to fail
  cat >"$1" <<STUB
#!/usr/bin/env bash
if [[ "\$(cat "$work/hang" 2>/dev/null)" == "$2" ]]; then
  sleep 300 &
  echo "\$!" >"$work/hung-pid"
  wait
fi
[[ "\$(cat "$work/fail" 2>/dev/null)" == "$2" ]] && { echo "stub failure: $2"; exit 1; }
exit 0
STUB
  chmod +x "$1"
}
stub "$tree/tools/policy/workspace_policy.sh" workspace-policy
stub "$tree/tools/policy/config_policy.sh" implicit-config
stub "$tree/tools/policy/bazelrc_policy.sh" bazelrc-policy
stub "$tree/tools/policy/lock_fixture.sh" toolchain-fixtures
stub "$tree/tools/hooks/e2e.sh" hook-e2e
stub "$tree/tools/speckit/bootstrap.sh" speckit-provenance
stub "$tree/tools/policy/from_base.sh" floors-from-base
stub "$work/bin/SUPPRESSION" suppression-ban
stub "$work/bin/FLOORS" coverage-floor
sed -i '$i echo "measurement: stub: 100.0% statements (1 of 1), floor 0.0%, branches unmeasured"' "$work/bin/FLOORS"
cat >"$work/bin/adapter" <<STUB
#!/usr/bin/env bash
if [[ "\$1" == "\$(cat "$work/hang" 2>/dev/null)" ]]; then
  sleep 300 &
  echo "\$!" >"$work/hung-pid"
  wait
fi
[[ "\$(cat "$work/fail" 2>/dev/null)" == "\$1" ]] && { echo "stub failure: \$1"; exit 1; }
exit 0
STUB
cat >"$tree/tools/bootstrap/preflight.sh" <<'STUB'
#!/usr/bin/env bash
echo isolation=full
STUB
tools=()
while read -r name; do
  if [[ "$name" != SUPPRESSION && "$name" != FLOORS ]]; then
    printf '#!/bin/sh\necho "1.0"\n' >"$work/bin/$name"
    chmod +x "$work/bin/$name"
  fi
  tools+=("--tool=$name=$work/bin/$name")
done < <(sed -nE 's/^tools \{ name: "([A-Z_]+)".*/\1/p' "$registry")
cat >"$tree/tools/bootstrap/bazel.sh" <<STUB
#!/usr/bin/env bash
case "\$1" in
  --path) echo "$work/bin/BUF" ;;
  run)
    if [[ -e "$tree/.verify/receipt.json.lock" ]] && ! flock -n "$tree/.verify/receipt.json.lock" true; then
      echo held >>"$work/lock-at-bazel"
    fi
    for arg in "\$@"; do
      if [[ "\$arg" == --script_path=* ]]; then
        printf '#!/bin/sh\nexport BUILD_WORKSPACE_DIRECTORY="%s"\nexec "%s" "--registry=%s" "--adapter=%s" %s "\$@"\n' \
          "$tree" "$runner" "$registry" "$work/bin/adapter" "${tools[*]}" >"\${arg#*=}"
        chmod +x "\${arg#*=}"
      fi
    done
    ;;
  test | coverage)
    # Honor the plan, so a wrong subtraction shows up as a wrong inventory.
    keep="$work/kept"
    cp "$labels" "\$keep"
    case " \$* " in
      *" //... "*) ;;
      *) grep -F '//web/e2e' "$labels" >"\$keep" ;;
    esac
    case " \$* " in *" -//web/e2e/... "*) sed -i '\\#//web/e2e#d' "\$keep" ;; esac
    selected="\$(cat "$work/fail" 2>/dev/null)"
    case "\$selected" in
      tests) echo '//:verify_body_test FAILED in 0.1s'; sed '/verify_body_test/d; s/\$/ PASSED in 0.1s/' "\$keep"; exit 3 ;;
      dependency-rules) echo '//bff:dependency_rules_test FAILED in 0.1s'; sed '/dependency_rules/d; s/\$/ PASSED in 0.1s/' "\$keep"; exit 3 ;;
      e2e-browser) echo '//web/e2e:notes_e2e_test FAILED in 0.1s'; sed '/notes_e2e/d; s/\$/ PASSED in 0.1s/' "\$keep"; exit 3 ;;
      test-inventory) sed 1d "\$keep" | sed 's/\$/ PASSED in 0.1s/' ;;
      *) sed 's/\$/ PASSED in 0.1s/' "\$keep" ;;
    esac
    if [[ "\$selected" == sandbox-evidence ]]; then echo 'INFO: 3 processes: 1 internal, 2 local.'
    else echo 'INFO: 3 processes: 1 internal, 2 linux-sandbox.'; fi
    ;;
esac
STUB
chmod +x "$work/bin/adapter" "$tree/tools/checks/"*.sh "$tree/tools/bootstrap/"*.sh "$tree/tools/verify.sh"

(
  cd "$tree"
  mkdir -p quality/checks/suppression/testdata tools/speckit
  printf '%064d  vendored.sh\n' 0 >tools/speckit/scaffold.sha256
  printf '%064d  fixture.go\n' 0 >quality/checks/suppression/testdata/fixtures.sha256
  touch vendored.sh fixture.go a.go a.ts BUILD.bazel .bazelrc api/a.proto .github/workflows/ci.yml
  printf '#!/bin/sh\n' >a.sh
)
GIT_CONFIG_GLOBAL=/dev/null git -C "$tree" init -q
git -C "$tree" add -A
commit() { GIT_CONFIG_GLOBAL=/dev/null git -C "$tree" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -q "$@"; }
commit -m base
# The policy base is an earlier commit that holds the evaluator's paths.
mkdir -p "$tree/quality/checks/coverage/cmd/floors"
touch "$tree/quality/checks/coverage/cmd/floors/main.go" "$tree/quality/coverage_floors.json"
echo 'escalate: "^quality/"' >"$tree/quality/registry.textproto"
git -C "$tree" add -A
commit -m evaluator
policy_base="$(git -C "$tree" rev-parse HEAD)"
commit --allow-empty -m tested

run() { HOME="$work" PATH="/usr/bin:/bin" bash "$tree/tools/verify.sh" "--base=$policy_base" "$@" </dev/null >"$work/out" 2>&1; }
receipt="$tree/.verify/receipt.json"
field() { grep -E "^  \"$1\": " "$receipt" | head -1 | sed -E 's/^[^:]*: *"?([^",]*)"?,?$/\1/'; }
state() { # the state of one check in the receipt
  grep -A1 "\"check\": \"$1\"" "$receipt" | sed -nE 's/.*"result": "([A-Z_]+)".*/\1/p' | head -1
}

mapfile -t expected < <(sed -nE 's/^  id: "([a-z0-9-]+)"/\1/p' "$registry")
[[ ${#expected[@]} -ge 20 ]] || fail "read only ${#expected[@]} check ids"

: >"$work/fail"
rc=0
run || rc=$?
[[ $rc -eq 0 ]] || { fail "all checks passing: exit $rc"; sed 's/^/  /' "$work/out" >&2; }
[[ "$(field result)" == PASSED ]] || fail "all checks passing: receipt result $(field result)"
[[ "$(field isolation)" == full ]] || fail "default isolation label"
for id in "${expected[@]}"; do
  [[ "$(state "$id")" == PASSED ]] || fail "$id: not recorded as PASSED in the control run, got '$(state "$id")'"
done

for id in "${expected[@]}"; do
  echo "$id" >"$work/fail"
  rc=0
  run || rc=$?
  [[ $rc -eq 1 ]] || fail "$id failing: exit $rc, want 1"
  [[ "$(field result)" == FAILED ]] || fail "$id failing: receipt result $(field result)"
  [[ "$(state "$id")" != PASSED ]] || fail "$id failing: the receipt records a pass"
done

# The two rows review asked to keep after the old body went: a required target Bazel never ran, and
# an action that ran outside the claimed sandbox, both beside a Bazel exit of 0.
echo test-inventory >"$work/fail"
run || true
[[ "$(state test-inventory)" == FAILED ]] || fail "an omitted required target was not caught"
[[ "$(state tests)" == PASSED ]] || fail "control: Bazel itself passed in that run"
echo sandbox-evidence >"$work/fail"
run || true
[[ "$(state sandbox-evidence)" == FAILED ]] || fail "actions outside the sandbox were not caught"

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
[[ $rc -eq 11 ]] || { fail "offline run: exit $rc, want 11"; sed 's/^/  /' "$work/out" >&2; }
[[ "$(field result)" != PASSED ]] || fail "an offline run must not report PASSED"
[[ "$(state speckit-provenance)" == SKIPPED ]] || fail "offline skip was not recorded"
[[ "$(state floors-from-base)" == ERROR ]] || fail "an offline run must not build a base"
grep -qF 'needs the network' "$receipt" || fail "the offline refusal named no cause"

run --weak-isolation || true
[[ "$(state sandbox-evidence)" == FAILED ]] || fail "a linux-sandbox summary under --weak-isolation must not match"

sed -i 's/^  run)$/  run) exit 1 ;;\n  never)/' "$tree/tools/bootstrap/bazel.sh"
rc=0
run || rc=$?
[[ $rc -eq 2 ]] || fail "an unbuildable runner: exit $rc, want 2"
grep -qF 'nothing was verified' "$work/out" || fail "an unbuildable runner must say nothing was verified"
sed -i 's/^  run) exit 1 ;;$/  run)/; /^  never)$/d' "$tree/tools/bootstrap/bazel.sh"

# The real runner owns the receipt below, through its own lock, so these cases hold if and only if the
# wrapper uses that protocol. The owner is hung on a check for the rest of its 20 s deadline.
lock="$receipt.lock"
held() { ! flock -n "$lock" true; }
same_as_sentinel() { [[ "$(cat "$1" 2>/dev/null)" == sentinel ]]; }
second() { HOME="$work" PATH="/usr/bin:/bin" bash "$tree/tools/verify.sh" "--base=$policy_base" </dev/null >"$work/second" 2>&1; }

echo format-go >"$work/hang"
: >"$work/fail"
rm -f "$work/hung-pid" "$work/lock-at-bazel"
run &
owner=$!
for _ in $(seq 1 100); do
  if [[ -s "$work/hung-pid" ]]; then break; fi
  sleep 0.1
done
[[ -s "$work/hung-pid" ]] || fail "control: the owner never reached its hung check"
echo sentinel >"$receipt"
held || fail "control: the real runner does not hold $lock"
rc=0
second || rc=$?
[[ $rc -eq 10 ]] || fail "a held receipt: exit $rc, want 10"
grep -qF 'another run holds' "$work/second" || fail "a held receipt: no refusal named"
held || fail "control: the owner let go during the refusal, so it proves nothing"
same_as_sentinel "$receipt" || fail "starting the wrapper removed a receipt its owner holds"

rc=0
wait "$owner" || rc=$?
[[ $rc -eq 124 ]] || fail "hung check: exit $rc, want 124"
grep -qF 'overall deadline' "$work/out" || fail "hung check: no diagnostic"
hung="$(cat "$work/hung-pid" 2>/dev/null || true)"
[[ -n "$hung" ]] || fail "the hang stub did not run"
sleep 1
if [[ -n "$hung" ]] && kill -0 "$hung" 2>/dev/null; then
  kill "$hung"
  fail "a grandchild of a bounded check outlived the overall deadline"
fi
if held; then fail "the receipt lock outlived its owner"; fi
[[ ! -e "$work/lock-at-bazel" ]] || fail "the receipt lock was held across a Bazel call"

# Once the owner has gone, startup still invalidates, even when that startup then fails.
: >"$work/hang"
echo sentinel >"$receipt"
sed -i 's/^echo isolation=full$/exit 1/' "$tree/tools/bootstrap/preflight.sh"
rc=0
run || rc=$?
[[ $rc -ne 0 ]] || fail "failed preflight passed after a release"
[[ ! -e "$receipt" ]] || fail "a stale receipt survived startup after its owner released"
sed -i 's/^exit 1$/echo isolation=full/' "$tree/tools/bootstrap/preflight.sh"

# A holder that lets go inside the bound is waited for: the receipt is intact until the release.
echo sentinel >"$receipt"
cat >"$work/hold.sh" <<STUB
touch "$work/hold-started"
sleep 1
cp "$receipt" "$work/at-release"
STUB
flock "$lock" bash "$work/hold.sh" &
holder=$!
for _ in $(seq 1 50); do
  if [[ -e "$work/hold-started" ]]; then break; fi
  sleep 0.1
done
[[ -e "$work/hold-started" ]] || fail "control: the short holder never held the lock"
rc=0
second || rc=$?
wait "$holder" || fail "control: the short holder failed"
[[ $rc -eq 0 ]] || { fail "waiting for a release: exit $rc, want 0"; sed 's/^/  /' "$work/second" >&2; }
same_as_sentinel "$work/at-release" || fail "the receipt was removed while the short holder held it"
[[ "$(field result)" == PASSED ]] || fail "after the wait: receipt result $(field result)"

# The overall deadline still rules a wait longer than itself.
sed 's/^deadline=[0-9]*$/deadline=1/' "$tree/tools/verify.sh" >"$tree/tools/verify_short.sh"
sed 's/^receipt_wait=[0-9]*$/receipt_wait=300/' "$tree/tools/checks/verify_body.sh" >"$tree/tools/checks/verify_body_long.sh"
sed -i 's/verify_body\.sh/verify_body_long.sh/' "$tree/tools/verify_short.sh"
echo sentinel >"$receipt"
flock "$lock" sleep 12 &
holder=$!
for _ in $(seq 1 50); do
  if held; then break; fi
  sleep 0.1
done
rc=0
HOME="$work" PATH="/usr/bin:/bin" bash "$tree/tools/verify_short.sh" </dev/null >"$work/second" 2>&1 || rc=$?
[[ $rc -eq 124 ]] || fail "a wait past the overall deadline: exit $rc, want 124"
same_as_sentinel "$receipt" || fail "the overall deadline removed a held receipt"
held || fail "control: the holder of the overall-deadline case let go too early"
kill "$holder" 2>/dev/null || true
wait "$holder" 2>/dev/null || true
for _ in $(seq 1 120); do
  if ! held; then break; fi
  sleep 0.1
done
rm -f "$tree/tools/verify_short.sh" "$tree/tools/checks/verify_body_long.sh"

# Without the declared locking tool nothing is removed unlocked, and the cause is named.
mkdir "$work/no-flock"
for tool in bash mktemp rm; do ln -s "$(command -v "$tool")" "$work/no-flock/$tool"; done
echo sentinel >"$receipt"
rc=0
env -i HOME="$work" PATH="$work/no-flock" "$work/no-flock/bash" "$tree/tools/checks/verify_body.sh" "$tree" \
  </dev/null >"$work/second" 2>&1 || rc=$?
[[ $rc -eq 2 ]] || fail "no flock: exit $rc, want 2"
grep -qF "bootstrap ERROR: 'flock' not found" "$work/second" || fail "no flock: the cause was not named"
same_as_sentinel "$receipt" || fail "no flock: the receipt was removed without the lock"

# Each leg runs its own checks and its own rows of the inventory. The whole run's results are the union.
: >"$work/fail"
: >"$work/hang"
rm -f "$receipt"
union="$work/union"
: >"$union"
mapfile -t legs < <(sed -nE 's/^shards: "([a-z0-9-]+)"$/\1/p' "$registry")
[[ ${#legs[@]} -eq 3 ]] || fail "read ${#legs[@]} legs"
for leg in "${legs[@]}"; do
  rc=0
  run "--shard=$leg" || rc=$?
  [[ $rc -eq 0 ]] || { fail "leg $leg: exit $rc"; sed 's/^/  /' "$work/out" >&2; }
  [[ "$(field shard)" == "$leg" ]] || fail "leg $leg: the receipt names '$(field shard)'"
  [[ "$(field base_revision)" == "$policy_base" ]] || fail "leg $leg: the receipt lost the base"
  [[ -n "$(field base_policy_digest)" ]] || fail "leg $leg: the receipt lost the base's digest"
  [[ "$(field result)" == PASSED ]] || fail "leg $leg: result $(field result)"
  grep -E '^      "check": ' "$receipt" | sed -E 's/.*"check": "([^"]+)".*/\1/' >>"$union"
done
for id in "${expected[@]}"; do
  grep -qFx "$id" "$union" || fail "no leg ran $id"
done
for postcondition in test-inventory sandbox-evidence; do
  [[ "$(grep -cFx "$postcondition" "$union")" -eq 2 ]] || fail "$postcondition did not run in each Bazel leg"
done
owned="$(sort "$union" | grep -vxE 'test-inventory|sandbox-evidence' | uniq -d)"
[[ -z "$owned" ]] || fail "an owned check ran in two legs: $owned"

# The real runner keeps the check's summary in the receipt.
: >"$work/fail"
run || fail "control before the summary case"
grep -qF 'stub: 100.0% statements (1 of 1), floor 0.0%, branches unmeasured' "$receipt" || fail "the receipt lost the coverage summary"

# With no base, every other check still reports, and the run is exit 11 with no pass.
rc=0
HOME="$work" PATH="/usr/bin:/bin" bash "$tree/tools/verify.sh" </dev/null >"$work/out" 2>&1 || rc=$?
[[ $rc -eq 11 ]] || fail "no base: exit $rc, want 11"
[[ "$(field result)" != PASSED ]] || fail "no base: the receipt says PASSED"
[[ "$(state floors-from-base)" == ERROR ]] || fail "no base: floors-from-base is '$(state floors-from-base)'"
[[ "$(state tests)" == PASSED && "$(state coverage-floor)" == PASSED ]] || fail "no base: other results were lost"
grep -qF 'no policy base was named' "$work/out" "$receipt" || fail "no base: the cause was not named"

# A leg that reads no base still cannot pass on a base it was given and could not use.
rc=0
HOME="$work" PATH="/usr/bin:/bin" bash "$tree/tools/verify.sh" --shard=e2e --base=no-such-ref </dev/null >"$work/out" 2>&1 || rc=$?
[[ $rc -eq 11 ]] || fail "a leg given a bad base: exit $rc, want 11"
[[ "$(field result)" != PASSED ]] || fail "a leg given a bad base: the receipt says PASSED"
[[ "$(state e2e-browser)" == PASSED ]] || fail "a leg given a bad base: its check result was lost"

# A leg's inventory is that leg's rows, so a target the leg did not run fails it.
echo test-inventory >"$work/fail"
rc=0
run --shard=e2e || rc=$?
[[ $rc -eq 1 && "$(state test-inventory)" == FAILED ]] || fail "a target missing from the e2e leg was not caught: exit $rc"
: >"$work/fail"
rc=0
run --shard=no-such-leg || rc=$?
[[ $rc -eq 2 ]] || fail "an unknown leg: exit $rc, want 2"

[[ $failures -eq 0 ]] && echo "ok   verify-body"
exit "$failures"
