#!/usr/bin/env bash
# Host capability preflight (specs/002-hermetic-skeleton FR-008, research R2;
# specs/005-browser-client FR-011).
#
#   tools/bootstrap/preflight.sh [--allow-weak-isolation]
#
# Exits 0 when the host can run the verifier at full isolation. With --allow-weak-isolation, an
# isolation failure is reported but does not fail; the caller must label its evidence `weakened`.
# Prints `isolation=full|unavailable` as its last line for the caller.
set -euo pipefail

allow_weak=false
if [[ "${1:-}" == "--allow-weak-isolation" ]]; then allow_weak=true; fi

# The pinned browser libraries are Debian 12 builds. libexpat and libstdc++ need GLIBC_2.36, and
# nothing in the closure needs newer. Keep README.md and web/README.md in step.
glibc_floor_major=2
glibc_floor_minor=36

failures=0
fail() { # fail <check> <cause> <remediation...>
  echo "preflight $1: $2" >&2
  shift 2
  for fix in "$@"; do echo "  fix: $fix" >&2; done
  failures=$((failures + 1))
}

check_glibc() {
  local floor="$glibc_floor_major.$glibc_floor_minor" reported major minor
  local fixes=(
    "use a Linux x86_64 host with glibc $floor or newer, such as Debian 12 or Ubuntu 24.04"
    "run in CI or a container with glibc $floor or newer"
    "the browser and its libraries are pinned; do not change them to fit this host"
  )
  reported="$(getconf GNU_LIBC_VERSION 2>/dev/null)" || reported=""
  if [[ ! "$reported" =~ ^glibc\ ([0-9]{1,4})\.([0-9]{1,4})(\.[0-9]{1,6})?$ ]]; then
    fail "libc" "cannot confirm glibc $floor or newer: getconf GNU_LIBC_VERSION reported '${reported:-nothing}'; other C libraries are unsupported" \
      "${fixes[@]}"
    return
  fi
  major="${BASH_REMATCH[1]}"
  minor="${BASH_REMATCH[2]}"
  if ((10#$major < glibc_floor_major || (10#$major == glibc_floor_major && 10#$minor < glibc_floor_minor))); then
    fail "libc" "$reported is below the $floor floor; the pinned browser libraries cannot load, so the browser tests would fail" \
      "${fixes[@]}"
  fi
}

for tool in bash git curl sha256sum timeout tar sort grep xargs flock; do
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

# macOS has no glibc. Its limits are recorded in docs/evidence/m2-m4.md.
if [[ "$(uname -s)" == "Linux" ]]; then check_glibc; fi
[[ $failures -eq 0 ]] || exit 1

isolation=full
if [[ "$(uname -s)" == "Linux" ]]; then
  helper="$(cd "$root" && tools/bootstrap/bazel.sh info install_base 2>/dev/null)/linux-sandbox"
  if [[ ! -x "$helper" ]] || ! timeout 30 "$helper" -- /bin/true </dev/null >/dev/null 2>&1; then
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
