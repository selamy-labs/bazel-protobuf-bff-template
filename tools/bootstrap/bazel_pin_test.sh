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
cp "$src" "$tree/tools/bootstrap/bazel.sh"
echo 9.9.9 >"$tree/.bazelversion"
cat >"$work/stub" <<'STUB'
#!/bin/sh
echo "args: $*"
echo "BAZELRC=${BAZELRC:-unset} JAVA_TOOL_OPTIONS=${JAVA_TOOL_OPTIONS:-unset}"
STUB
sha="$(sha256sum "$work/stub" | cut -d' ' -f1)"
case "$(uname -s)-$(uname -m)" in
  Linux-x86_64) platform=linux-x86_64 ;;
  Linux-aarch64) platform=linux-arm64 ;;
  Darwin-arm64) platform=darwin-arm64 ;;
esac
pin() { printf '%s %s %s\n' "$platform" "$1" "$2" >"$tree/tools/bootstrap/bazel.pins"; }
run() { "$tree/tools/bootstrap/bazel.sh" "$@"; }

pin "$sha" "file://$work/stub"
out="$(BAZELRC=/hostile JAVA_TOOL_OPTIONS=-Dhostile=1 run info)" || fail "pinned binary did not run"
grep -qF -- 'args: --nosystem_rc --nohome_rc info' <<<"$out" || fail "rc isolation flags missing: $out"
grep -qF -- 'BAZELRC=unset JAVA_TOOL_OPTIONS=unset' <<<"$out" || fail "ambient env reached Bazel: $out"
[[ "$(run --path)" == "$XDG_CACHE_HOME/bff-template/bazel/9.9.9-$platform-$sha/bazel" ]] || fail "--path"

printf '#!/bin/sh\necho tampered\n' >"$(run --path)"
if out="$(run info 2>&1)"; then fail "tampered cached binary ran: $out"; fi
grep -qF 'checksum mismatch for cached' <<<"$out" || fail "tampered cache diagnostic: $out"

rm -rf "$XDG_CACHE_HOME"
pin "$(printf 'other' | sha256sum | cut -d' ' -f1)" "file://$work/stub"
if out="$(run info 2>&1)"; then fail "wrong checksum accepted: $out"; fi
grep -qF 'checksum mismatch for downloaded' <<<"$out" || fail "download diagnostic: $out"
[[ -z "$(find "$XDG_CACHE_HOME" -type f 2>/dev/null)" ]] || fail "rejected download left a file behind"

pin "$sha" "https://example.invalid/bazel/1.2.3/bazel"
if out="$(run info 2>&1)"; then fail "URL for another version accepted: $out"; fi
grep -qF 'does not match .bazelversion' <<<"$out" || fail "version mismatch diagnostic: $out"

pin "$sha" "file://$work/missing"
if out="$(run info 2>&1)"; then fail "failed download accepted: $out"; fi
grep -qF 'download failed' <<<"$out" || fail "download failure diagnostic: $out"

[[ $failures -eq 0 ]] && echo "ok   bazel-pin"
exit "$failures"
