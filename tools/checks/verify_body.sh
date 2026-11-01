#!/usr/bin/env bash
# The body of tools/verify.sh, run in its scrubbed environment. Do not call it directly. It proves
# the host can run, builds the runner, and hands over: the registry says what the full policy is.
set -euo pipefail
root="$1"
shift
cd "$root"
receipt=.verify/receipt.json
receipt_wait=30

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# timeout gives its command a process group, so the wrapper's overall deadline reaches it only
# through this handler, which passes the signal on.
active=""
trap 'if [[ -n "$active" ]]; then kill -TERM "$active" 2>/dev/null || true; fi; exit 143' TERM

bounded() { # bounded <seconds> <command...>: stdin is closed; returns the command's exit code
  local seconds="$1" rc=0
  shift
  timeout --kill-after=60 "$seconds" "$@" </dev/null &
  active=$!
  wait "$active" || rc=$?
  active=""
  return "$rc"
}

# A receipt on disk always describes the run that just finished, never an earlier one. One producer
# owns a receipt path, through <receipt>.lock, for its whole run (Run in quality/runner/stage.go), so
# this removal takes that same flock. It is held for the rm alone: no shell descriptor keeps it, and
# --close keeps it from rm, so nothing that starts Bazel can carry it into a daemon.
invalidate_receipt() {
  local rc=0
  if ! command -v flock >/dev/null 2>&1; then
    echo "verify: bootstrap ERROR: 'flock' not found in /usr/bin or /bin; install util-linux. No stale receipt was removed and nothing was verified" >&2
    exit 2
  fi
  mkdir -p "$(dirname "$receipt")"
  bounded "$((receipt_wait + 10))" flock --close --wait "$receipt_wait" --conflict-exit-code 75 \
    "$receipt.lock" rm -f "$receipt" || rc=$?
  case "$rc" in
    0) ;;
    75 | 124 | 137)
      echo "verify: another run holds $receipt; it was left in place and nothing was verified" >&2
      exit 10
      ;;
    *)
      echo "verify: could not remove the stale $receipt (exit $rc); nothing was verified" >&2
      exit 2
      ;;
  esac
}
invalidate_receipt

weak=false
offline=false
bootstrap_only=false
for arg in "$@"; do
  case "$arg" in
    --weak-isolation) weak=true ;;
    --offline) offline=true ;;
    --bootstrap-only) bootstrap_only=true ;;
    *) echo "verify: unknown argument '$arg'" >&2; exit 2 ;;
  esac
done

bazel_flags=()
runner_flags=(--stage=full "--receipt=$root/$receipt")
rerun="tools/verify.sh"
preflight_args=()
if $weak; then
  bazel_flags+=(--config=weak-isolation)
  runner_flags+=(--isolation=weakened)
  rerun="tools/verify.sh --weak-isolation"
  preflight_args+=(--allow-weak-isolation)
fi
if $offline; then
  bazel_flags+=(--repository_disable_download)
  runner_flags+=(--offline)
  rerun+=" --offline"
fi

if ! bounded 600 tools/bootstrap/preflight.sh "${preflight_args[@]}" >"$work/preflight.log" 2>&1; then
  cat "$work/preflight.log" >&2
  echo "verify: preflight failed; nothing was verified" >&2
  exit 1
fi

if ! bounded 1200 tools/bootstrap/bazel.sh run "${bazel_flags[@]}" "--script_path=$work/runner" \
  --noshow_progress --ui_event_filters=-info,-stdout,-stderr //:quality >"$work/bootstrap.log" 2>&1; then
  tail -n 30 "$work/bootstrap.log" >&2
  echo "verify: could not build the runner; nothing was verified (rerun: $rerun)" >&2
  exit 2
fi
if $bootstrap_only; then
  echo "verify: toolchain ready"
  exit 0
fi

rc=0
bounded 2340 "$work/runner" "${runner_flags[@]}" "--rerun=$rerun" || rc=$?
if [[ $rc -eq 124 || $rc -eq 137 ]]; then
  echo "verify: the runner did not stop on its own; nothing was verified" >&2
fi
exit "$rc"
