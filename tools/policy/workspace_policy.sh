#!/usr/bin/env bash
# Rejects legacy WORKSPACE dependency files (specs/002-hermetic-skeleton FR-002).
# Reads a NUL-delimited repository path list from $1.
set -euo pipefail
found="$(tr '\0' '\n' <"$1" | grep -E '(^|/)WORKSPACE(\.bazel|\.bzlmod)?$' || true)"
if [[ -n "$found" ]]; then
  while IFS= read -r path; do
    echo "bzlmod-policy $path:1: legacy WORKSPACE file; declare dependencies in MODULE.bazel (rerun: bazel run //:verify)"
  done <<<"$found"
  exit 1
fi
