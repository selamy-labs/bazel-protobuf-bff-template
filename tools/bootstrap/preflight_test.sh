#!/usr/bin/env bash
# Witnesses the glibc floor in preflight.sh (specs/005-browser-client FR-011). Runs the real script;
# only what the host reports is stubbed.
set -euo pipefail

fail() {
  echo "preflight_test: $*" >&2
  exit 1
}

tree="$(mktemp -d "${TEST_TMPDIR:-/tmp}/preflight.XXXXXX")"
mkdir -p "$tree/tools/bootstrap" "$tree/bin" "$tree/install"
cp tools/bootstrap/preflight.sh "$tree/tools/bootstrap/"

cat >"$tree/tools/bootstrap/bazel.sh" <<STUB
#!/usr/bin/env bash
touch "$tree/bazel-started"
echo "$tree/install"
STUB
printf '#!/usr/bin/env bash\nexit 0\n' >"$tree/install/linux-sandbox"
cat >"$tree/bin/uname" <<'STUB'
#!/usr/bin/env bash
echo "${FAKE_UNAME:-Linux}"
STUB
printf '#!/usr/bin/env bash\necho header\necho "fs 1 1 99999999999 1%% /"\n' >"$tree/bin/df"
cat >"$tree/bin/getconf" <<'STUB'
#!/usr/bin/env bash
[[ "$1" == GNU_LIBC_VERSION ]] || exit 2
if [[ -n "${FAKE_LIBC:-}" ]]; then printf '%s\n' "$FAKE_LIBC"; fi
exit "${FAKE_LIBC_RC:-0}"
STUB
chmod +x "$tree/tools/bootstrap/bazel.sh" "$tree/install/linux-sandbox" "$tree"/bin/*

run() { # run <libc report> <getconf status> [preflight args...]; sets rc and output
  local libc="$1" status="$2"
  shift 2
  rm -f "$tree/bazel-started"
  rc=0
  output="$(FAKE_LIBC="$libc" FAKE_LIBC_RC="$status" PATH="$tree/bin:$PATH" \
    "$tree/tools/bootstrap/preflight.sh" "$@" 2>&1)" || rc=$?
}

accepted() { # accepted <libc report>
  run "$1" 0
  [[ $rc -eq 0 ]] || fail "'$1' was refused: $output"
  [[ "$(tail -n 1 <<<"$output")" == isolation=full ]] || fail "'$1' lost the isolation line: $output"
  [[ -e "$tree/bazel-started" ]] || fail "'$1' did not reach the isolation probe"
}

refused() { # refused <expected text>; judges the latest run
  [[ $rc -ne 0 ]] || fail "'$libc_case' passed: $output"
  grep -qF "$1" <<<"$output" || fail "'$libc_case' did not say '$1': $output"
  grep -qF 'fix: use a Linux x86_64 host with glibc 2.36 or newer' <<<"$output" ||
    fail "'$libc_case' gave no way forward: $output"
  [[ ! -e "$tree/bazel-started" ]] || fail "'$libc_case' started Bazel before failing"
  if grep -q '^isolation=' <<<"$output"; then fail "'$libc_case' reported isolation: $output"; fi
}

# At and above the floor. 2.100 fails a text comparison of versions.
for libc_case in 'glibc 2.36' 'glibc 2.36.9000' 'glibc 2.39' 'glibc 2.100' 'glibc 3.0'; do
  accepted "$libc_case"
done

# Below the floor. 2.4 passes a text comparison, and 2.35 is the first real version under it.
for libc_case in 'glibc 2.35' 'glibc 2.4' 'glibc 1.99'; do
  run "$libc_case" 0
  refused "$libc_case is below the 2.36 floor"
done

# Weak isolation is about the sandbox. It does not make the browser load.
libc_case='glibc 2.35 with weak isolation'
run 'glibc 2.35' 0 --allow-weak-isolation
refused 'glibc 2.35 is below the 2.36 floor'

# Unknown never passes: not glibc, no answer, or an answer that can't be read.
for libc_case in 'musl libc 1.2.5' 'glibc stable' 'glibc 2.36-custom' $'glibc 2.36\nglibc 2.1'; do
  run "$libc_case" 0
  refused 'cannot confirm glibc 2.36 or newer'
done
libc_case='getconf cannot answer'
run '' 1
refused "reported 'nothing'"
libc_case='getconf is absent'
run '' 127
refused "reported 'nothing'"

# macOS has no glibc and is not held to the floor, even when getconf has no answer.
rc=0
output="$(FAKE_UNAME=Darwin FAKE_LIBC_RC=1 PATH="$tree/bin:$PATH" \
  "$tree/tools/bootstrap/preflight.sh" 2>&1)" || rc=$?
[[ $rc -eq 0 ]] || fail "macOS was held to the floor: $output"

for doc in README.md web/README.md; do
  grep -qF 'glibc 2.36 or' "$doc" || fail "$doc no longer states the floor"
done

echo "preflight_test: PASSED"
