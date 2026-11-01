#!/usr/bin/env bash
# Prints NAME=absolute-path for each pinned tool or file (NAME ends in _FILE) passed as
# NAME=runfiles-path. Run with `bazel run`;
# the verifier then calls the binaries directly from the repository root.
set -euo pipefail
for pair in "$@"; do
  name="${pair%%=*}"
  path="${pair#*=}"
  case "$name" in
    *_FILE) [[ -f "$path" ]] || { echo "tool-paths: $name is missing: $path" >&2; exit 1; } ;;
    *) [[ -x "$path" ]] || { echo "tool-paths: $name is not executable: $path" >&2; exit 1; } ;;
  esac
  # The verifier sources this output, and a checkout path can hold any character.
  printf '%s=%q\n' "$name" "$(realpath "$path")"
done
# A js_binary finds its Node and packages through this tree, wherever it is called from.
printf 'RUNFILES_ROOT=%q\n' "$(realpath ..)"
