#!/usr/bin/env bash
# Lockfile witness (specs/002-hermetic-skeleton US1 scenario 2): an unlocked MODULE.bazel edit
# must fail under the repository's .bazelrc, and the unmodified copy must pass. Runs Bazel in a
# throwaway copy with its own output root, sharing only the download cache.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
work="$(mktemp -d)"
trap '(cd "$work/ws" 2>/dev/null && bazel --output_user_root="$work/out" shutdown) >/dev/null 2>&1; rm -rf "$work"' EXIT
cache="$(cd "$root" && bazel info repository_cache)"

mkdir -p "$work/ws"
cp "$root"/{.bazelversion,.bazelrc,MODULE.bazel,MODULE.bazel.lock,REPO.bazel} "$work/ws/"
cp -r "$root/quality" "$work/ws/quality"
touch "$work/ws/BUILD.bazel"

mod_deps() { (cd "$work/ws" && bazel --output_user_root="$work/out" mod deps --repository_cache="$cache" </dev/null >"$work/log" 2>&1); }

mod_deps || { echo "lock-fixture: unmodified copy failed:" >&2; tail -5 "$work/log" >&2; exit 1; }
# Bazel 9 locks registry file hashes and extension results, not the resolved graph, so the
# witness adds a module version whose registry files the lock has never recorded.
echo 'bazel_dep(name = "bazel_skylib", version = "1.9.2")' >>"$work/ws/MODULE.bazel"
if mod_deps; then
  echo "lock-fixture: unlocked MODULE.bazel edit was accepted" >&2
  exit 1
fi
grep -qi 'lockfile' "$work/log" || { echo "lock-fixture: failure did not name the lockfile:" >&2; tail -5 "$work/log" >&2; exit 1; }
echo "ok   lock-fixture defect and corrected"
