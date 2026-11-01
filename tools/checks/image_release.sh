#!/usr/bin/env bash
# image-release (spec 009 FR-006, FR-007): the canonical image is the plain build's. The stage's own Bazel
# invocation may be `coverage`, whose transitioned output directories differ from a plain build's, so its
# image differs in digest (spec 009 research R6). This check builds and witnesses the image in the plain
# configuration, after the stage's invocation, and reports the plain digest as the identity a runtime binds.
#
#   image_release.sh <Bazel wrapper> <full|weakened>
#
# Exit 0 passed, 1 a finding, 11 nothing could be judged.
set -euo pipefail
wrapper="$1"
isolation="$2"
refuse() { echo "image-release: $1" >&2; exit 11; }
[[ -x "$wrapper" ]] || refuse "no Bazel wrapper at $wrapper"
flags=(--noshow_progress)
if [[ "$isolation" == weakened ]]; then flags+=(--config=weak-isolation); fi
"$wrapper" test "${flags[@]}" --test_output=errors //bff/cmd/bff:image_test //bff/cmd/bff:image </dev/null || exit 1
layout="$("$wrapper" cquery "${flags[@]}" --output=files //bff/cmd/bff:image </dev/null 2>/dev/null | head -1)"
[[ -n "$layout" && -f "$layout/index.json" ]] || refuse "the plain image layout was not found"
digest="$(python3 -I -c 'import json, sys; print(json.load(open(sys.argv[1]))["manifests"][0]["digest"])' "$layout/index.json")"
hash="${digest#sha256:}"
[[ "$(sha256sum "$layout/blobs/sha256/$hash" | cut -d' ' -f1)" == "$hash" ]] || { echo "image-release: the manifest's bytes do not hash to $digest"; exit 1; }
echo "measurement: image $digest, plain configuration, witnessed by //bff/cmd/bff:image_test"
