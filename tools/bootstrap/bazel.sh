#!/usr/bin/env bash
# Runs the pinned Bazel (specs/002-hermetic-skeleton T030). The binary's sha256 is checked on every
# call, and host/user rc files and environment overrides cannot change the verdict.
#
#   tools/bootstrap/bazel.sh [startup options] <command> [options]
#
# --path prints the verified binary instead of running it.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
releases="https://github.com/bazelbuild/bazel/releases/download"
version="$(<"$root/.bazelversion")"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "bazel-pin: .bazelversion '$version' is not a release version" >&2
  exit 2
fi

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=linux-x86_64 ;;
  Linux-aarch64) platform=linux-arm64 ;;
  Darwin-arm64) platform=darwin-arm64 ;;
  *) echo "bazel-pin: no pinned Bazel for $(uname -s)-$(uname -m); see tools/bootstrap/bazel.pins" >&2; exit 2 ;;
esac

read -r _ sha256 < <(grep -E "^$platform " "$root/tools/bootstrap/bazel.pins") ||
  { echo "bazel-pin: no entry for $platform in tools/bootstrap/bazel.pins" >&2; exit 2; }
url="$releases/$version/bazel-$version-$platform"

cache="${XDG_CACHE_HOME:-$HOME/.cache}/bff-template/bazel/$version-$platform-$sha256"
bin="$cache/bazel"
matches() { [[ "$(sha256sum "$1" | cut -d' ' -f1)" == "$sha256" ]]; }

if [[ ! -f "$bin" ]]; then
  mkdir -p "$cache"
  tmp="$(mktemp "$cache/download.XXXXXX")"
  if ! curl -q --fail --silent --show-error --location --retry 3 --connect-timeout 30 --max-time 600 \
    --output "$tmp" "$url" </dev/null; then
    rm -f "$tmp"
    echo "bazel-pin: download failed: $url" >&2
    exit 1
  fi
  if ! matches "$tmp"; then
    rm -f "$tmp"
    echo "bazel-pin: checksum mismatch for downloaded $url (expected $sha256); not using it" >&2
    exit 1
  fi
  chmod +x "$tmp"
  mv "$tmp" "$bin"
fi
if ! matches "$bin"; then
  echo "bazel-pin: checksum mismatch for cached $bin (expected $sha256); delete it and rerun" >&2
  exit 1
fi

if [[ "${1:-}" == "--path" ]]; then
  echo "$bin"
  exit 0
fi
exec env -u BAZELRC -u JAVA_TOOL_OPTIONS -u JDK_JAVA_OPTIONS -u _JAVA_OPTIONS \
  "$bin" --nosystem_rc --nohome_rc "$@"
