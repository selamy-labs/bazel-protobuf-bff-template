#!/usr/bin/env bash
# The canonical verifier (specs/002-hermetic-skeleton FR-004..FR-007). CI runs exactly this.
#
#   tools/verify.sh                   full policy at full isolation
#   tools/verify.sh --weak-isolation  explicit opt-in for hosts without namespace sandboxing; every
#                                     result is labeled `isolation: weakened`
#   tools/verify.sh --offline         use only already-fetched artifacts, skip the networked Spec Kit
#                                     provenance check, and refuse floors-from-base. It never passes,
#                                     and CI never uses it
#   tools/verify.sh --bootstrap-only  fetch and verify the pinned toolchain, then stop
#   tools/verify.sh --shard=<leg>     one leg of the policy, as one CI job runs it
#   tools/verify.sh --base=<rev>      the policy base to compare with, resolved once. It says nothing
#                                     about whether anyone reviewed that commit
#
# The whole run has one overall deadline, inside CI's job timeout. Each phase also has its own.
#
# This wrapper discards the caller's environment, so rc files, tool options and PATH helpers
# cannot change a verdict. Only what is listed here reaches the verifier. Network proxy settings
# pass through because they affect how pinned artifacts are fetched, never what they are.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
deadline=2400

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
  GITHUB_RUN_ID GITHUB_RUN_ATTEMPT GITHUB_WORKFLOW GITHUB_JOB; do
  if [[ -n "${!name:-}" ]]; then keep+=("$name=${!name}"); fi
done
rc=0
env -i "${keep[@]}" timeout --kill-after=30 "$deadline" bash "$root/tools/checks/verify_body.sh" "$root" "$@" || rc=$?
if [[ $rc -eq 124 || $rc -eq 137 ]]; then
  echo "verify: overall deadline of ${deadline}s exceeded; the verifier was killed and proved nothing" >&2
fi
exit "$rc"
