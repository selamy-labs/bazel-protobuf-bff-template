#!/usr/bin/env bash
# Required real-image acceptance (spec009 T018/T019). No registry, cloud or daemon changes.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
: "${BFF_IMAGE_RUNTIME_OUTPUTS:?owned receipt artifact directory is required}"
mkdir -p -- "$BFF_IMAGE_RUNTIME_OUTPUTS"
BFF_IMAGE_RUNTIME_OUTPUTS="$(realpath -- "$BFF_IMAGE_RUNTIME_OUTPUTS")"
export BFF_IMAGE_RUNTIME_OUTPUTS
BFF_IMAGE_REVISION="$(git rev-parse --verify HEAD)"
export BFF_IMAGE_REVISION
export TEST_UNDECLARED_OUTPUTS_DIR="$BFF_IMAGE_RUNTIME_OUTPUTS"
export E2E_REPORT="$BFF_IMAGE_RUNTIME_OUTPUTS/report.json"
# Bind the exact checkout and dependency inputs; a dirty tracked candidate cannot claim this revision.
git diff --quiet HEAD -- .
sha256sum MODULE.bazel MODULE.bazel.lock go.mod go.sum pnpm-lock.yaml .bazelversion >"$BFF_IMAGE_RUNTIME_OUTPUTS/source-inputs.sha256"
printf '%s\n' "$BFF_IMAGE_REVISION" >"$BFF_IMAGE_RUNTIME_OUTPUTS/source-revision.txt"
status=0
tools/bootstrap/bazel.sh run //web/e2e:image_runtime >"$BFF_IMAGE_RUNTIME_OUTPUTS/runtime-command.log" 2>&1 || status=$?
printf '{"exit":%s,"witness":"actual-local-container","source":"%s"}\n' "$status" "$BFF_IMAGE_REVISION" >"$BFF_IMAGE_RUNTIME_OUTPUTS/command-result.json"
cat "$BFF_IMAGE_RUNTIME_OUTPUTS/runtime-command.log"
if [[ "$status" != 0 ]]; then exit "$status"; fi
# Emit the accepted image identity only after every actual-container test and teardown passes.
tools/bootstrap/bazel.sh build --noshow_progress @bff_template_quality//tools:yq
parser="$(tools/bootstrap/bazel.sh cquery --output=files @bff_template_quality//tools:yq 2>/dev/null | head -1)"
digest="$("$parser" -p json '.manifestDigest' "$BFF_IMAGE_RUNTIME_OUTPUTS/image-identity.json")"
[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo 'image-runtime: accepted manifest identity missing' >&2; exit 1; }
echo "measurement: runtime image $digest"
