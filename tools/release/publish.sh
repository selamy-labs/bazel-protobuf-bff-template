#!/usr/bin/env bash
# Trusted publishing (spec 010 FR-013): on the tagged revision, already verified by the workflow's verify job,
# build the image through the build graph, publish it by digest to the repository-linked container package with
# the job's own token, and attach the release artifacts with their hashes. Nothing is rebuilt by another builder;
# the digest pushed is compared with the digest the plain build measures, and a mismatch ends the run.
#
#   tools/release/publish.sh <owner/repository> <tag>      # needs GITHUB_TOKEN; run only on the trusted source
set -euo pipefail
repository="$1"; tag="$2"
[[ -n "${GITHUB_TOKEN:-}" ]] || { echo "publish: GITHUB_TOKEN is not set; this runs only in the publishing job" >&2; exit 2; }
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "publish: $tag is not a release tag" >&2; exit 2; }
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
revision="$(git rev-parse HEAD)"
[[ "$(git rev-parse "refs/tags/$tag^{commit}")" == "$revision" ]] || { echo "publish: $tag does not name the checked-out revision" >&2; exit 2; }
package="ghcr.io/${repository,,}/bff"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
# The plain build and its digest, measured from content by the same witness the verifier runs.
tools/bootstrap/bazel.sh build --noshow_progress "--ui_event_filters=-info,-stdout,-stderr" //bff/cmd/bff:image //bff/cmd/bff:image_tarball --output_groups=+tarball
digest="$(tools/checks/image_release.sh | sed -nE 's/^measurement: image (sha256:[0-9a-f]{64}),.*/\1/p')"
[[ -n "$digest" ]] || { echo "publish: the plain build's digest could not be measured" >&2; exit 2; }
# The push, with the job's token; the registry answers with the digest it stored, which must be the measured one.
printf '%s' "$GITHUB_TOKEN" | docker login ghcr.io -u "${GITHUB_ACTOR:-token}" --password-stdin >/dev/null
tools/bootstrap/bazel.sh run --noshow_progress "--ui_event_filters=-info,-stdout,-stderr" //bff/cmd/bff:image_push -- --repository "$package" --tag "$tag"
pushed="$(docker manifest inspect --verbose "$package@$digest" >/dev/null 2>&1 && echo "$digest" || true)"
[[ "$pushed" == "$digest" ]] || { echo "publish: the registry does not serve $digest after the push" >&2; exit 1; }
# The release: the tarball, the receipt, the inventory, the notices, and the hashes of each.
tarball="$(tools/bootstrap/bazel.sh cquery --output=files --output_groups=tarball //bff/cmd/bff:image_tarball 2>/dev/null | tail -1)"
cp "$tarball" "$work/bff-image-$tag.tar"
cp .verify/receipt.json "$work/verify-receipt-$tag.json" 2>/dev/null || echo "publish: no receipt beside the build; the verify job's artifact is the receipt" >&2
cp docs/provenance.md "$work/provenance-$tag.md"
cp LICENSE "$work/LICENSE"
printf '%s\n' "$revision" >"$work/source-revision-$tag.txt"
printf '%s\n' "$digest" >"$work/image-digest-$tag.txt"
(cd "$work" && sha256sum ./* >SHA256SUMS)
gh release create "$tag" --repo "$repository" --verify-tag --prerelease --title "$tag" \
  --notes "Pre-release $tag at $revision. Image $package@$digest. Not a stable release; the quality baseline is open (spec 011)." \
  "$work"/*
echo "publish: $package@$digest and the assets of $tag at $revision"
