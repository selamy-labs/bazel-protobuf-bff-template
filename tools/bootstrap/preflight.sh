#!/usr/bin/env bash
# Host capability preflight (specs/002-hermetic-skeleton FR-008, research R2).
#
#   tools/bootstrap/preflight.sh [--allow-weak-isolation]
#
# Exits 0 when the host can run the verifier at full isolation. With --allow-weak-isolation, an
# isolation failure is reported but does not fail; the caller must label its evidence `weakened`.
# Prints `isolation=full|unavailable` as its last line for the caller.
set -euo pipefail

allow_weak=false
if [[ "${1:-}" == "--allow-weak-isolation" ]]; then allow_weak=true; fi

failures=0
fail() { # fail <check> <cause> <remediation...>
  echo "preflight $1: $2" >&2
  shift 2
  for fix in "$@"; do echo "  fix: $fix" >&2; done
  failures=$((failures + 1))
}

for tool in bash git curl sha256sum timeout tar sort grep xargs; do
  command -v "$tool" >/dev/null 2>&1 ||
    fail "prerequisite" "'$tool' not found in /usr/bin or /bin" "install it with your OS package manager"
done
[[ $failures -eq 0 ]] || exit 1

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
free_kb="$(df -Pk "$root" | awk 'NR==2 {print $4}')"
if ((free_kb < 10 * 1024 * 1024)); then
  fail "disk" "$((free_kb / 1024 / 1024)) GB free under $root; at least 10 GB is needed" \
    "free disk space on this volume"
fi

isolation=full
if [[ "$(uname -s)" == "Linux" ]]; then
  helper="$(cd "$root" && tools/bootstrap/bazel.sh info install_base 2>/dev/null)/linux-sandbox"
  if [[ ! -x "$helper" ]] || ! "$helper" -- /bin/true </dev/null >/dev/null 2>&1; then
    isolation=unavailable
    cause="Bazel's linux-sandbox cannot create user/mount namespaces"
    if [[ "$(cat /proc/sys/kernel/apparmor_restrict_unprivileged_userns 2>/dev/null)" == "1" ]]; then
      cause+=" (kernel.apparmor_restrict_unprivileged_userns=1)"
    fi
    fixes=(
      "[admin] add an AppArmor profile granting 'userns,' to \$HOME/.cache/bazel/_bazel_*/install/*/linux-sandbox (narrowest; now $helper)"
      "[admin] sysctl -w kernel.apparmor_restrict_unprivileged_userns=0 (host-wide; weaker)"
      "run in CI or a container where unprivileged user namespaces are allowed"
      "[no admin] tools/verify.sh --weak-isolation (results are labeled 'isolation: weakened')"
    )
    if $allow_weak; then
      echo "preflight isolation: $cause; continuing with explicitly requested weak isolation" >&2
    else
      fail "isolation" "$cause" "${fixes[@]}"
    fi
  fi
fi

echo "isolation=$isolation"
[[ $failures -eq 0 ]]
