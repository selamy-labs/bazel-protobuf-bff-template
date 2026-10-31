#!/usr/bin/env bash
# Rejects legacy WORKSPACE dependency files (specs/002-hermetic-skeleton FR-002) and committed
# generated code (specs/003-notes-contract FR-003).
# Reads a NUL-delimited repository path list from $1.
set -euo pipefail
found="$(tr '\0' '\n' <"$1" | grep -E '(^|/)WORKSPACE(\.bazel|\.bzlmod)?$' || true)"
if [[ -n "$found" ]]; then
  while IFS= read -r path; do
    echo "bzlmod-policy $path:1: legacy WORKSPACE file; declare dependencies in MODULE.bazel (rerun: tools/verify.sh)"
  done <<<"$found"
  exit 1
fi
generated="$(tr '\0' '\n' <"$1" | grep -E '(\.pb\.go|\.connect\.go|_pb\.ts)$' || true)"
if [[ -n "$generated" ]]; then
  while IFS= read -r path; do
    echo "generated-code $path:1: generated code comes from the build graph; delete this file (rerun: tools/verify.sh)"
  done <<<"$generated"
  exit 1
fi
