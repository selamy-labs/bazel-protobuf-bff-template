#!/usr/bin/env bash
# Witnesses for the pinned Bazel wrapper (specs/002-hermetic-skeleton T030, US1 scenario 5), using
# a stub binary served from file:// so no download is needed.
set -euo pipefail
src="$PWD/tools/bootstrap/bazel.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
export XDG_CACHE_HOME="$work/cache"
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }

tree="$work/tree"
mkdir -p "$tree/tools/bootstrap"
# Only the release host is replaced, so the URL construction under test stays the real one.
releases="file://$work/releases"
sed "s|^releases=.*|releases=\"$releases\"|" "$src" >"$tree/tools/bootstrap/bazel.sh"
chmod +x "$tree/tools/bootstrap/bazel.sh"
cmp -s "$src" "$tree/tools/bootstrap/bazel.sh" && fail "the release host was not found in bazel.sh"
echo 9.9.9 >"$tree/.bazelversion"
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=linux-x86_64 ;;
  Linux-aarch64) platform=linux-arm64 ;;
  Darwin-arm64) platform=darwin-arm64 ;;
esac
stub="$work/releases/9.9.9/bazel-9.9.9-$platform"
mkdir -p "$(dirname "$stub")"
cat >"$stub" <<'STUB'
#!/bin/sh
echo "args: $*"
echo "BAZELRC=${BAZELRC:-unset} JAVA_TOOL_OPTIONS=${JAVA_TOOL_OPTIONS:-unset}"
STUB
sha="$(sha256sum "$stub" | cut -d' ' -f1)"
pin() { printf '%s %s\n' "$platform" "$1" >"$tree/tools/bootstrap/bazel.pins"; }
run() { "$tree/tools/bootstrap/bazel.sh" "$@"; }

pin "$sha"
out="$(BAZELRC=/hostile JAVA_TOOL_OPTIONS=-Dhostile=1 run info)" || fail "pinned binary did not run"
grep -qF -- 'args: --nosystem_rc --nohome_rc info' <<<"$out" || fail "rc isolation flags missing: $out"
grep -qF -- 'BAZELRC=unset JAVA_TOOL_OPTIONS=unset' <<<"$out" || fail "ambient env reached Bazel: $out"
[[ "$(run --path)" == "$XDG_CACHE_HOME/bff-template/bazel/9.9.9-$platform-$sha/bazel" ]] || fail "--path"

printf '#!/bin/sh\necho tampered\n' >"$(run --path)"
if out="$(run info 2>&1)"; then fail "tampered cached binary ran: $out"; fi
grep -qF 'checksum mismatch for cached' <<<"$out" || fail "tampered cache diagnostic: $out"

rm -rf "$XDG_CACHE_HOME"
pin "$(printf 'other' | sha256sum | cut -d' ' -f1)"
if out="$(run info 2>&1)"; then fail "wrong checksum accepted: $out"; fi
grep -qF 'checksum mismatch for downloaded' <<<"$out" || fail "download diagnostic: $out"
[[ -z "$(find "$XDG_CACHE_HOME" -type f 2>/dev/null)" ]] || fail "rejected download left a file behind"

pin "$sha"
for bad in "" "9.9" "9.9.9/../../x"; do
  printf '%s\n' "$bad" >"$tree/.bazelversion"
  if out="$(run info 2>&1)"; then fail "version '$bad' accepted: $out"; fi
  grep -qF 'not a release version' <<<"$out" || fail "version '$bad' diagnostic: $out"
done
echo 9.9.8 >"$tree/.bazelversion"
if out="$(run info 2>&1)"; then fail "failed download accepted: $out"; fi
grep -qF 'download failed' <<<"$out" || fail "download failure diagnostic: $out"
echo 9.9.9 >"$tree/.bazelversion"

mkdir -p "$work/curlhome"
printf 'fail-with-body-this-option-does-not-exist\n' >"$work/curlhome/.curlrc"
HOME="$work/curlhome" run info >/dev/null 2>&1 || fail "a hostile ~/.curlrc reached the download"

[[ $failures -eq 0 ]] && echo "ok   bazel-pin"
exit "$failures"
