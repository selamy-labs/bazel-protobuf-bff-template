#!/usr/bin/env bash
# Offline refusal and binding controls for tools/checks/image_runtime.sh (#33).
# Verifies dirty tracked/untracked refusal, maintained bootstrap/parser custody (ignoring env overrides),
# failed witness preservation, stale receipt removal, independent identity/terminal tree+revision+digest+teardown
# receipt verification, and exact measurement output.
set -euo pipefail
script="$(realpath "$1")"
yq="$(realpath "$2")"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

repo="$work/repo"
mkdir -p "$repo/tools/checks" "$repo/tools/bootstrap"
cp "$script" "$repo/tools/checks/image_runtime.sh"
chmod +x "$repo/tools/checks/image_runtime.sh"

for f in MODULE.bazel MODULE.bazel.lock go.mod go.sum pnpm-lock.yaml .bazelversion; do
  printf '%s-fixture\n' "$f" >"$repo/$f"
done

cat >"$repo/tools/bootstrap/bazel.sh" <<STUB
#!/usr/bin/env bash
set -euo pipefail
if [[ -n "\${STUB_CALLED_FILE:-}" ]]; then
  printf '%s\n' "\$*" >>"\$STUB_CALLED_FILE"
fi
if [[ "\${1:-}" == "build" ]]; then
  exit 0
fi
if [[ "\${1:-}" == "cquery" ]]; then
  printf '%s\n' "$yq"
  exit 0
fi
if [[ "\${1:-}" != "run" || "\${2:-}" != "//web/e2e:image_runtime" ]]; then
  echo "unexpected bazel.sh invocation: \$*" >&2
  exit 2
fi
echo "stub image_runtime (exit=\${STUB_EXIT:-0})"
if [[ "\${STUB_EXIT:-0}" != 0 ]]; then
  exit "\${STUB_EXIT}"
fi
identity_rev="\${STUB_IDENTITY_REV:-\$BFF_IMAGE_REVISION}"
term_rev="\${STUB_TERM_REV:-\$BFF_IMAGE_REVISION}"
identity_tree="\${STUB_IDENTITY_TREE:-\$BFF_IMAGE_TREE}"
term_tree="\${STUB_TERM_TREE:-\$BFF_IMAGE_TREE}"
manifest="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
config="sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
term_manifest="\${STUB_TERM_MANIFEST:-\$manifest}"
term_config="\${STUB_TERM_CONFIG:-\$config}"
dirty="\${STUB_DIRTY:-false}"
teardown="\${STUB_TEARDOWN:-true}"
remaining="\${STUB_REMAINING:-0}"
printf '{"revision":"%s","sourceTree":"%s","manifestDigest":"%s","configDigest":"%s"}\n' \
  "\$identity_rev" "\$identity_tree" "\$manifest" "\$config" >"\$BFF_IMAGE_RUNTIME_OUTPUTS/image-identity.json"
if [[ "\${STUB_SKIP_TERMINAL:-0}" != 1 ]]; then
  printf '{"schemaVersion":1,"revision":"%s","sourceTree":"%s","sourceDirty":%s,"manifestDigest":"%s","configDigest":"%s","dockerHost":"unix:///var/run/docker.sock","serverVersion":"29.1.3","serverApiVersion":"1.52","ownedContainersRemaining":%s,"teardownVerified":%s}\n' \
    "\$term_rev" "\$term_tree" "\$dirty" "\$term_manifest" "\$term_config" "\$remaining" "\$teardown" >"\$BFF_IMAGE_RUNTIME_OUTPUTS/runtime-terminal.json"
fi
STUB
chmod +x "$repo/tools/bootstrap/bazel.sh"

git -C "$repo" init -q
git -C "$repo" config user.name "Test"
git -C "$repo" config user.email "test@example.invalid"
git -C "$repo" add .
git -C "$repo" commit -q -m "fixture"
head_rev="$(git -C "$repo" rev-parse HEAD)"
head_tree="$(git -C "$repo" rev-parse 'HEAD^{tree}')"

failures=0
expect_fail() {
  local label="$1" expected_err="$2"
  shift 2
  local rc=0 out
  out="$("$@" 2>&1)" || rc=$?
  if [[ "$rc" -ne 0 && "$out" == *"$expected_err"* ]]; then
    echo "ok   $label"
  else
    echo "FAIL $label: rc=$rc out=$out"
    failures=$((failures + 1))
  fi
}

# 1. Missing output directory env var is refused.
expect_fail "missing output env" "owned receipt artifact directory is required" \
  env -u BFF_IMAGE_RUNTIME_OUTPUTS "$repo/tools/checks/image_runtime.sh"

# 2. Dirty tracked file is refused before running witness.
printf 'dirty-tracked\n' >>"$repo/go.mod"
expect_fail "dirty tracked checkout" "tracked or untracked changes" \
  env BFF_IMAGE_RUNTIME_OUTPUTS="$work/dirty-tracked" "$repo/tools/checks/image_runtime.sh"
git -C "$repo" checkout -- go.mod

# 3. Dirty untracked file is refused before running witness.
printf 'untracked\n' >"$repo/stray.txt"
expect_fail "dirty untracked checkout" "tracked or untracked changes" \
  env BFF_IMAGE_RUNTIME_OUTPUTS="$work/dirty-untracked" "$repo/tools/checks/image_runtime.sh"
rm -f "$repo/stray.txt"

# 3b. Failed git status (unreadable index yielding empty stdout + nonzero exit) refuses before witness starts.
bad_index_dir="$work/bad-index-dir"
mkdir -p "$bad_index_dir"
status_called="$work/status-fail-called"
expect_fail "failed git status refuses before witness" "git status failed" \
  env GIT_INDEX_FILE="$bad_index_dir" STUB_CALLED_FILE="$status_called" \
  BFF_IMAGE_RUNTIME_OUTPUTS="$work/status-fail" "$repo/tools/checks/image_runtime.sh"
if [[ -e "$status_called" ]]; then
  echo "FAIL failed git status still invoked bazel witness"
  failures=$((failures + 1))
else
  echo "ok   failed git status never started witness"
fi

# 4. Failing runtime witness preserves failure exit + command-result.json and removes stale receipts.
fail_out="$work/fail-out"
mkdir -p "$fail_out"
printf '{"stale":true}\n' >"$fail_out/image-identity.json"
printf '{"stale":true}\n' >"$fail_out/runtime-terminal.json"
rc=0
STUB_EXIT=7 BFF_IMAGE_RUNTIME_OUTPUTS="$fail_out" \
  "$repo/tools/checks/image_runtime.sh" >/dev/null 2>&1 || rc=$?
if [[ "$rc" -eq 7 && ! -e "$fail_out/image-identity.json" && ! -e "$fail_out/runtime-terminal.json" ]] && \
   grep -q "\"exit\":7,\"witness\":\"actual-local-container\",\"source\":\"$head_rev\",\"tree\":\"$head_tree\",\"dirty\":false" "$fail_out/command-result.json"; then
  echo "ok   failed runtime witness clears stale receipts and records exit"
else
  echo "FAIL failed runtime witness: rc=$rc"
  failures=$((failures + 1))
fi

# 5. Environment overrides for alternate Bazel or yq binaries are ignored (maintained wrapper still fails).
expect_fail "env override cannot bypass maintained bazel wrapper" "stub image_runtime (exit=7)" \
  env STUB_EXIT=7 BFF_IMAGE_RUNTIME_BAZEL=/bin/true BFF_IMAGE_RUNTIME_YQ=/bin/true \
  BFF_IMAGE_RUNTIME_OUTPUTS="$work/override-attempt" "$repo/tools/checks/image_runtime.sh"

# 6. Missing runtime-terminal.json is refused.
expect_fail "missing terminal receipt" "accepted runtime receipts missing" \
  env STUB_SKIP_TERMINAL=1 BFF_IMAGE_RUNTIME_OUTPUTS="$work/missing-term" "$repo/tools/checks/image_runtime.sh"

# 7. Mismatched revision in identity or terminal receipt is refused.
expect_fail "mismatched identity revision" "revision does not match" \
  env STUB_IDENTITY_REV="9999999999999999999999999999999999999999" BFF_IMAGE_RUNTIME_OUTPUTS="$work/bad-id-rev" "$repo/tools/checks/image_runtime.sh"
expect_fail "mismatched terminal revision" "revision does not match" \
  env STUB_TERM_REV="9999999999999999999999999999999999999999" BFF_IMAGE_RUNTIME_OUTPUTS="$work/bad-term-rev" "$repo/tools/checks/image_runtime.sh"

# 8. Mismatched sourceTree in image-identity.json alone OR runtime-terminal.json alone is refused.
expect_fail "mismatched identity sourceTree only" "source tree or cleanliness mismatch" \
  env STUB_IDENTITY_TREE="7777777777777777777777777777777777777777" BFF_IMAGE_RUNTIME_OUTPUTS="$work/bad-id-tree" "$repo/tools/checks/image_runtime.sh"
expect_fail "mismatched terminal sourceTree only" "source tree or cleanliness mismatch" \
  env STUB_TERM_TREE="8888888888888888888888888888888888888888" BFF_IMAGE_RUNTIME_OUTPUTS="$work/bad-term-tree" "$repo/tools/checks/image_runtime.sh"

# 9. Dirty flag in terminal receipt is refused.
expect_fail "dirty terminal receipt" "source tree or cleanliness mismatch" \
  env STUB_DIRTY=true BFF_IMAGE_RUNTIME_OUTPUTS="$work/bad-dirty" "$repo/tools/checks/image_runtime.sh"

# 10. Mismatched terminal manifest or config digest is refused.
expect_fail "mismatched manifest digest" "does not match image identity" \
  env STUB_TERM_MANIFEST="sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" BFF_IMAGE_RUNTIME_OUTPUTS="$work/bad-manifest" "$repo/tools/checks/image_runtime.sh"
expect_fail "mismatched config digest" "does not match image identity" \
  env STUB_TERM_CONFIG="sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd" BFF_IMAGE_RUNTIME_OUTPUTS="$work/bad-config" "$repo/tools/checks/image_runtime.sh"

# 11. Non-zero ownedContainersRemaining or unverified teardown is refused.
expect_fail "remaining owned container" "teardown verification failed" \
  env STUB_REMAINING=1 BFF_IMAGE_RUNTIME_OUTPUTS="$work/bad-remain" "$repo/tools/checks/image_runtime.sh"
expect_fail "unverified teardown" "teardown verification failed" \
  env STUB_TEARDOWN=false BFF_IMAGE_RUNTIME_OUTPUTS="$work/bad-teardown" "$repo/tools/checks/image_runtime.sh"

# 12. Clean passing witness emits exact measurement and preserves source-revision.txt and source-tree.txt.
pass_out="$work/pass-out"
pass_stdout="$(BFF_IMAGE_RUNTIME_OUTPUTS="$pass_out" "$repo/tools/checks/image_runtime.sh")"
if grep -qx 'measurement: runtime image sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' <<<"$pass_stdout" && \
   [[ "$(cat "$pass_out/source-revision.txt")" == "$head_rev" ]] && \
   [[ "$(cat "$pass_out/source-tree.txt")" == "$head_tree" ]]; then
  echo "ok   clean passing witness binds revision, tree, digests, and teardown"
else
  echo "FAIL clean passing witness: $pass_stdout"
  failures=$((failures + 1))
fi

exit "$failures"
