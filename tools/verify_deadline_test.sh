#!/usr/bin/env bash
# Witness for the verifier's overall deadline (specs/002-hermetic-skeleton T032). A copy of the real
# wrapper, with its deadline shortened, runs a stub body that hangs with a child process.
set -euo pipefail
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/tree/tools/checks"
sed 's/^deadline=[0-9]*$/deadline=1/' tools/verify.sh >"$work/tree/tools/verify.sh"
cmp -s tools/verify.sh "$work/tree/tools/verify.sh" && { echo "FAIL the deadline was not found" >&2; exit 1; }
cat >"$work/tree/tools/checks/verify_body.sh" <<STUB
#!/usr/bin/env bash
sleep 300 &
echo "\$!" >"$work/child"
wait
STUB
rc=0
start="$(date +%s)"
bash "$work/tree/tools/verify.sh" >"$work/out" 2>&1 </dev/null || rc=$?
elapsed=$(($(date +%s) - start))
[[ $rc -eq 124 ]] || { echo "FAIL exit $rc, want 124" >&2; cat "$work/out" >&2; exit 1; }
grep -qF 'overall deadline' "$work/out" || { echo "FAIL no diagnostic" >&2; cat "$work/out" >&2; exit 1; }
[[ $elapsed -lt 30 ]] || { echo "FAIL took ${elapsed}s" >&2; exit 1; }
child="$(cat "$work/child")"
# The signal has gone out, but a dying process stays visible for a moment and a zombie still answers
# kill -0, so a loaded host needs a bounded wait. A child that escaped would still be here at its end.
running() {
  local state
  state="$(ps -o stat= -p "$1" 2>/dev/null || true)"
  [[ -n "$state" && "$state" != Z* ]]
}
for _ in $(seq 1 50); do
  if ! running "$child"; then break; fi
  sleep 0.1
done
if running "$child"; then
  kill "$child"
  echo "FAIL the hung child outlived the deadline" >&2
  exit 1
fi
echo "ok   verify-deadline"
