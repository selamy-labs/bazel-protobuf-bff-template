#!/usr/bin/env bash
# The canonical verifier (specs/002-hermetic-skeleton FR-004..FR-007). CI runs exactly this.
#
#   tools/verify.sh                   full policy at full isolation
#   tools/verify.sh --weak-isolation  explicit opt-in for hosts without namespace sandboxing; every
#                                     result is labeled `isolation: weakened`
#   tools/verify.sh --offline         use only already-fetched artifacts and skip the networked
#                                     Spec Kit provenance check (CI never passes this)
#   tools/verify.sh --bootstrap-only  fetch and verify the pinned toolchain, then stop
#
# This wrapper discards the caller's environment, so rc files, tool options and PATH helpers
# cannot change a verdict. Only what is listed here reaches the verifier. Network proxy settings
# pass through because they affect how pinned artifacts are fetched, never what they are.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

keep=(
  "HOME=$HOME"
  "USER=${USER:-$(id -un)}"
  "PATH=/usr/bin:/bin"
  "TMPDIR=${TMPDIR:-/tmp}"
  "LC_ALL=C.UTF-8"
  "TERM=dumb"
  "GIT_CONFIG_GLOBAL=/dev/null"
  "GIT_CONFIG_SYSTEM=/dev/null"
  "GIT_TERMINAL_PROMPT=0"
)
for name in HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy SSL_CERT_FILE SSL_CERT_DIR \
  XDG_CACHE_HOME GITHUB_RUN_ID GITHUB_RUN_ATTEMPT GITHUB_WORKFLOW GITHUB_JOB; do
  if [[ -n "${!name:-}" ]]; then keep+=("$name=${!name}"); fi
done
exec env -i "${keep[@]}" bash "$root/tools/checks/verify_body.sh" "$root" "$@"
