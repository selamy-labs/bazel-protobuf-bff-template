#!/usr/bin/env bash
# Runs the pinned ORAS release binary. Archive and executable SHA-256 digests are checked against
# tools/release/oras.pins on download and before every invocation, with zero ambient PATH fallback.
#
#   tools/release/oras.sh [--path | <oras args...>]
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
releases="https://github.com/oras-project/oras/releases/download"

case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=linux_amd64 ;;
  *) echo "oras-pin: no pinned ORAS for $(uname -s)-$(uname -m); see tools/release/oras.pins" >&2; exit 2 ;;
esac

pin_line="$(grep -E "^[0-9]+\.[0-9]+\.[0-9]+ $platform " "$root/tools/release/oras.pins" | head -n 1 || true)"
[[ -n "$pin_line" ]] || { echo "oras-pin: no entry for $platform in tools/release/oras.pins" >&2; exit 2; }
read -r version _ archive_sha binary_sha <<<"$pin_line"
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ || ! "$archive_sha" =~ ^[0-9a-f]{64}$ || ! "$binary_sha" =~ ^[0-9a-f]{64}$ ]]; then
  echo "oras-pin: invalid pin entry for $platform in tools/release/oras.pins" >&2
  exit 2
fi

url="$releases/v$version/oras_${version}_${platform}.tar.gz"
cache="${XDG_CACHE_HOME:-$HOME/.cache}/bff-template/oras/$version-$platform-$binary_sha"
bin="$cache/oras"
matches() { [[ "$(sha256sum -- "$1" | cut -d' ' -f1)" == "$2" ]]; }

tmp_dir=""
cleanup() { [[ -z "$tmp_dir" ]] || rm -rf -- "$tmp_dir"; }
trap cleanup EXIT

if [[ -L "$bin" ]]; then
  echo "oras-pin: cached binary at $bin must not be a symlink" >&2
  exit 1
fi

if [[ ! -f "$bin" ]]; then
  mkdir -p -- "$cache"
  tmp_dir="$(mktemp -d "$cache/download.XXXXXX")"
  if ! env -u GITHUB_TOKEN -u GH_TOKEN curl -q --fail --silent --show-error --location \
    --proto '=https' --tlsv1.2 --retry 3 --connect-timeout 30 --max-time 300 \
    --output "$tmp_dir/archive.tar.gz" "$url" </dev/null; then
    echo "oras-pin: download failed: $url" >&2
    exit 1
  fi
  if ! matches "$tmp_dir/archive.tar.gz" "$archive_sha"; then
    echo "oras-pin: checksum mismatch for downloaded $url (expected $archive_sha); not using it" >&2
    exit 1
  fi
  (umask 077 && tar -xzf "$tmp_dir/archive.tar.gz" -C "$tmp_dir" oras)
  if [[ -L "$tmp_dir/oras" || ! -f "$tmp_dir/oras" ]] || ! matches "$tmp_dir/oras" "$binary_sha"; then
    echo "oras-pin: checksum mismatch for extracted binary (expected $binary_sha); not using it" >&2
    exit 1
  fi
  chmod 0755 "$tmp_dir/oras"
  mv "$tmp_dir/oras" "$bin"
  rm -rf -- "$tmp_dir"
  tmp_dir=""
fi

if [[ -L "$bin" || ! -f "$bin" ]] || ! matches "$bin" "$binary_sha"; then
  echo "oras-pin: checksum mismatch for cached $bin (expected $binary_sha); delete it and rerun" >&2
  exit 1
fi

if [[ "${1:-}" == "--path" ]]; then
  echo "$bin"
  exit 0
fi

exec env -u GITHUB_TOKEN -u GH_TOKEN "$bin" "$@"
