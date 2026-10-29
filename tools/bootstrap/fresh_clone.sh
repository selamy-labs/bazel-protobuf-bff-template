#!/usr/bin/env bash
# Fresh-clone witness (specs/002-hermetic-skeleton FR-011, SC-001).
#
#   tools/bootstrap/fresh_clone.sh [--weak-isolation]
#
# Clones HEAD into an empty directory whose path contains a space, with an empty HOME so no Bazel,
# Bazelisk or uv cache and no credential or Git configuration is visible, then runs the canonical
# verifier. Prints the clone's receipt.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
revision="$(git -C "$root" rev-parse HEAD)"
weak=()
config=()
if [[ "${1:-}" == "--weak-isolation" ]]; then
  weak=(-- --weak-isolation)
  config=(--config=weak-isolation)
fi

work="$(mktemp -d)"
home="$work/home"
clone="$work/fresh clone"
mkdir -p "$home"
trap '(cd "$clone" 2>/dev/null && HOME="$home" bazel shutdown) >/dev/null 2>&1; chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT

git clone --quiet --no-local "file://$root" "$clone"
git -C "$clone" checkout --quiet "$revision"

rc=0
(cd "$clone" && env -i HOME="$home" PATH="$PATH" TERM=dumb USER="${USER:-}" \
  GIT_CONFIG_NOSYSTEM=1 GIT_TERMINAL_PROMPT=0 \
  bazel run "${config[@]}" //:verify "${weak[@]}" </dev/null) || rc=$?

echo "fresh-clone: revision $revision, exit $rc"
cat "$clone/.verify/receipt.json" 2>/dev/null || echo "fresh-clone: no receipt was written" >&2
exit "$rc"
