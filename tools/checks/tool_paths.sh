#!/usr/bin/env bash
# Prints NAME=absolute-path for each pinned tool passed as NAME=runfiles-path. Run with `bazel run`;
# the verifier then calls the binaries directly from the repository root.
set -euo pipefail
for pair in "$@"; do
  name="${pair%%=*}"
  path="${pair#*=}"
  [[ -x "$path" ]] || { echo "tool-paths: $name is not executable: $path" >&2; exit 1; }
  printf '%s=%s\n' "$name" "$(realpath "$path")"
done
