#!/usr/bin/env bash
# Installs the hooks with the pinned prek (spec 006 T012, FR-015). Run it with
#
#   tools/bootstrap/bazel.sh run //tools/hooks:install [-- --weak-isolation]
#
# prek's shim records the path of the prek that installed it and, if that path is gone, falls back to
# any prek on PATH. A path in Bazel's outputs disappears with `bazel clean`, so the pinned binary is
# copied beside the repository's Git data first and installs itself from there. This is a pin, not a
# wrapper: chaining foreign hooks, worktrees and core.hooksPath are prek's own.
set -euo pipefail
prek="$(realpath "$1")"
shift
isolation=full
for arg in "$@"; do
  case "$arg" in
    --weak-isolation) isolation=weakened ;;
    *) echo "install: unknown argument '$arg'" >&2; exit 2 ;;
  esac
done
cd "${BUILD_WORKSPACE_DIRECTORY:?run this with bazel run}"
common="$(git rev-parse --path-format=absolute --git-common-dir)"
bin="$common/bff-template/bin"
mkdir -p "$bin"
copy="$(mktemp "$bin/prek.XXXXXX")"
cp "$prek" "$copy"
chmod 755 "$copy"
mv "$copy" "$bin/prek"
"$bin/prek" install --hook-type pre-commit --hook-type pre-push
git config bff-template.isolation "$isolation"
echo "install: hooks use $bin/prek (isolation: $isolation)"
