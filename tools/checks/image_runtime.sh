#!/usr/bin/env bash
# Required real-image acceptance (spec009 T018/T019, #33). No registry, cloud or daemon changes.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
: "${BFF_IMAGE_RUNTIME_OUTPUTS:?owned receipt artifact directory is required}"
BFF_IMAGE_REVISION="$(git rev-parse --verify HEAD)"
BFF_IMAGE_TREE="$(git rev-parse --verify 'HEAD^{tree}')"
export BFF_IMAGE_REVISION
export BFF_IMAGE_TREE
# Bind the exact checkout (tracked and untracked) and dependency inputs; a dirty candidate cannot claim this revision.
status_out="$(git status --porcelain=v1 -uall)" || {
  echo "image-runtime: git status failed; cannot verify clean checkout for $BFF_IMAGE_REVISION" >&2
  exit 1
}
if [[ -n "$status_out" ]]; then
  echo "image-runtime: checkout has tracked or untracked changes; a dirty candidate cannot claim $BFF_IMAGE_REVISION" >&2
  exit 1
fi
mkdir -p -- "$BFF_IMAGE_RUNTIME_OUTPUTS"
BFF_IMAGE_RUNTIME_OUTPUTS="$(realpath -- "$BFF_IMAGE_RUNTIME_OUTPUTS")"
export BFF_IMAGE_RUNTIME_OUTPUTS
export TEST_UNDECLARED_OUTPUTS_DIR="$BFF_IMAGE_RUNTIME_OUTPUTS"
export E2E_REPORT="$BFF_IMAGE_RUNTIME_OUTPUTS/report.json"
rm -f -- \
  "$BFF_IMAGE_RUNTIME_OUTPUTS/image-identity.json" \
  "$BFF_IMAGE_RUNTIME_OUTPUTS/runtime-terminal.json" \
  "$BFF_IMAGE_RUNTIME_OUTPUTS/command-result.json"
sha256sum MODULE.bazel MODULE.bazel.lock go.mod go.sum pnpm-lock.yaml .bazelversion >"$BFF_IMAGE_RUNTIME_OUTPUTS/source-inputs.sha256"
printf '%s\n' "$BFF_IMAGE_REVISION" >"$BFF_IMAGE_RUNTIME_OUTPUTS/source-revision.txt"
printf '%s\n' "$BFF_IMAGE_TREE" >"$BFF_IMAGE_RUNTIME_OUTPUTS/source-tree.txt"
status=0
tools/bootstrap/bazel.sh run //web/e2e:image_runtime >"$BFF_IMAGE_RUNTIME_OUTPUTS/runtime-command.log" 2>&1 || status=$?
printf '{"exit":%s,"witness":"actual-local-container","source":"%s","tree":"%s","dirty":false}\n' \
  "$status" "$BFF_IMAGE_REVISION" "$BFF_IMAGE_TREE" >"$BFF_IMAGE_RUNTIME_OUTPUTS/command-result.json"
cat "$BFF_IMAGE_RUNTIME_OUTPUTS/runtime-command.log"
if [[ "$status" != 0 ]]; then exit "$status"; fi
# Emit the accepted image identity only after every actual-container test and verified teardown passes.
tools/bootstrap/bazel.sh build --noshow_progress @bff_template_quality//tools:yq
parser="$(tools/bootstrap/bazel.sh cquery --output=files @bff_template_quality//tools:yq 2>/dev/null | head -1)"
[[ -x "$parser" ]] || { echo 'image-runtime: yq parser missing' >&2; exit 1; }
identity_file="$BFF_IMAGE_RUNTIME_OUTPUTS/image-identity.json"
terminal_file="$BFF_IMAGE_RUNTIME_OUTPUTS/runtime-terminal.json"
[[ -f "$identity_file" && -f "$terminal_file" ]] || { echo 'image-runtime: accepted runtime receipts missing' >&2; exit 1; }
digest="$("$parser" -p json '.manifestDigest' "$identity_file")"
config_digest="$("$parser" -p json '.configDigest' "$identity_file")"
identity_rev="$("$parser" -p json '.revision' "$identity_file")"
identity_tree="$("$parser" -p json '.sourceTree' "$identity_file")"
term_digest="$("$parser" -p json '.manifestDigest' "$terminal_file")"
term_config="$("$parser" -p json '.configDigest' "$terminal_file")"
term_rev="$("$parser" -p json '.revision' "$terminal_file")"
term_tree="$("$parser" -p json '.sourceTree' "$terminal_file")"
term_dirty="$("$parser" -p json '.sourceDirty' "$terminal_file")"
term_teardown="$("$parser" -p json '.teardownVerified' "$terminal_file")"
term_remaining="$("$parser" -p json '.ownedContainersRemaining' "$terminal_file")"
[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ && "$config_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || {
  echo 'image-runtime: accepted manifest or config identity missing' >&2
  exit 1
}
[[ "$identity_rev" == "$BFF_IMAGE_REVISION" && "$term_rev" == "$BFF_IMAGE_REVISION" ]] || {
  echo 'image-runtime: receipt revision does not match checked-out HEAD' >&2
  exit 1
}
[[ "$identity_tree" == "$BFF_IMAGE_TREE" && "$term_tree" == "$BFF_IMAGE_TREE" && "$term_dirty" == false ]] || {
  echo 'image-runtime: receipt source tree or cleanliness mismatch' >&2
  exit 1
}
[[ "$term_digest" == "$digest" && "$term_config" == "$config_digest" ]] || {
  echo 'image-runtime: terminal receipt digest does not match image identity' >&2
  exit 1
}
[[ "$term_teardown" == true && "$term_remaining" == 0 ]] || {
  echo 'image-runtime: owned container teardown verification failed' >&2
  exit 1
}
echo "measurement: runtime image $digest"
