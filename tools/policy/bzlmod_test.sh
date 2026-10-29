#!/usr/bin/env bash
# Witnesses for the Bzlmod policy (specs/002-hermetic-skeleton T007).
set -euo pipefail
policy="$1"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
printf 'MODULE.bazel\0src/BUILD.bazel\0' >"$work/good"
printf 'MODULE.bazel\0third_party/WORKSPACE.bazel\0' >"$work/bad"
"$policy" "$work/good" >/dev/null || { echo "FAIL: clean path list rejected" >&2; exit 1; }
if out="$("$policy" "$work/bad")"; then
  echo "FAIL: WORKSPACE.bazel accepted" >&2
  exit 1
fi
grep -q '^bzlmod-policy third_party/WORKSPACE.bazel:1: ' <<<"$out" || { echo "FAIL: diagnostic: $out" >&2; exit 1; }
echo "ok   bzlmod-policy defect and corrected"
