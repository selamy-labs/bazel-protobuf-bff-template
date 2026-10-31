#!/usr/bin/env bash
# Toolchain witnesses that need a real Bazel (specs/002-hermetic-skeleton US1, T030, T032):
#   1. an unlocked MODULE.bazel edit fails under the repository's .bazelrc, and the unmodified copy
#      passes;
#   2. a native tool whose checksum does not match its pin is rejected, not used;
#   3. a hostile HOME rc breaks raw Bazel (control) but cannot reach the pinned wrapper.
# Runs in a throwaway copy with its own output root, sharing only the download cache.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
bazel="$root/tools/bootstrap/bazel.sh"
work="$(mktemp -d)"
trap '(cd "$work/ws" 2>/dev/null && "$bazel" --output_user_root="$work/out" shutdown) >/dev/null 2>&1; chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT
cache="$(cd "$root" && "$bazel" info repository_cache)"

mkdir -p "$work/ws"
cp "$root"/{.bazelversion,.bazelrc,MODULE.bazel,MODULE.bazel.lock,REPO.bazel} "$work/ws/"
cp -r "$root/quality" "$work/ws/quality"
touch "$work/ws/BUILD.bazel"

in_ws() { (cd "$work/ws" && "$bazel" --output_user_root="$work/out" "$@" --repository_cache="$cache" </dev/null >"$work/log" 2>&1); }

in_ws mod deps || { echo "lock-fixture: unmodified copy failed:" >&2; tail -5 "$work/log" >&2; exit 1; }
cp "$work/ws/MODULE.bazel" "$work/MODULE.clean"
# Bazel 9 locks registry file hashes and extension results, not the resolved graph, so the witness
# adds a module version whose registry files the lock has never recorded.
echo 'bazel_dep(name = "bazel_skylib", version = "1.9.2")' >>"$work/ws/MODULE.bazel"
if in_ws mod deps; then
  echo "lock-fixture: unlocked MODULE.bazel edit was accepted" >&2
  exit 1
fi
grep -qi 'lockfile' "$work/log" || { echo "lock-fixture: failure did not name the lockfile:" >&2; tail -5 "$work/log" >&2; exit 1; }
cp "$work/MODULE.clean" "$work/ws/MODULE.bazel"
echo "ok   lock-fixture defect and corrected"

printf 'not the pinned tool\n' >"$work/tampered.tar.gz"
wrong="$(printf 'something else' | sha256sum | cut -d' ' -f1)"
"$YQ" -i -o=json "(.shellcheck.binaries[]) |= (.url = \"file://$work/tampered.tar.gz\" | .sha256 = \"$wrong\")" "$work/ws/quality/multitool.lock.json"
# update mode keeps the lock's registry hashes, so this needs no registry access; the tampered tool
# is fetched from file://, and only its checksum can fail.
if in_ws build --config=weak-isolation --nobuild --lockfile_mode=update @bff_template_quality//tools:shellcheck; then
  echo "checksum-fixture: a tool with the wrong checksum was accepted" >&2
  exit 1
fi
grep -qi 'checksum' "$work/log" || { echo "checksum-fixture: failure did not name the checksum:" >&2; tail -5 "$work/log" >&2; exit 1; }
echo "ok   checksum-fixture defect"

mkdir -p "$work/hostile"
echo 'common --option_that_only_a_hostile_rc_would_set' >"$work/hostile/.bazelrc"
binary="$("$bazel" --path)"
cached="${XDG_CACHE_HOME:-$HOME/.cache}"
if (cd "$work/ws" && HOME="$work/hostile" "$binary" --batch --output_user_root="$work/out2" version </dev/null >"$work/log" 2>&1); then
  echo "ambient-fixture: control failed; raw Bazel ignored the hostile rc, so this proves nothing" >&2
  exit 1
fi
grep -qF 'option_that_only_a_hostile_rc_would_set' "$work/log" || { echo "ambient-fixture: control failed for another reason:" >&2; tail -3 "$work/log" >&2; exit 1; }
for hostile_env in "HOME=$work/hostile" "BAZELRC=$work/hostile/.bazelrc"; do
  (cd "$work/ws" && env "XDG_CACHE_HOME=$cached" "$hostile_env" "$bazel" --batch --output_user_root="$work/out2" version </dev/null >"$work/log" 2>&1) ||
    { echo "ambient-fixture: $hostile_env reached the pinned wrapper:" >&2; tail -3 "$work/log" >&2; exit 1; }
done
echo "ok   ambient-fixture control and corrected"
