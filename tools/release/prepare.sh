#!/usr/bin/env bash
# Credentialless release artifact producer (spec 010 FR-013): builds the single plain image and
# tarball once without credentials, witnesses the actual container runtime without credentials,
# packs the public release assets alongside a digest-attested OCI layout archive, and verifies
# the handoff bundle before handing off to the separate write-scoped publish job.
#
#   tools/release/prepare.sh <tag> <handoff-dir>
set -euo pipefail
[[ $# -eq 2 ]] || { echo "prepare: usage: prepare.sh <tag> <handoff-dir>" >&2; exit 2; }
tag="$1"
handoff_arg="$2"
[[ -z "${GITHUB_TOKEN:-}" && -z "${GH_TOKEN:-}" ]] || { echo "prepare: runs without publication credentials, by design" >&2; exit 2; }
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "prepare: $tag is not a release tag" >&2; exit 2; }
[[ -n "$handoff_arg" ]] || { echo "prepare: handoff directory is required" >&2; exit 2; }

handoff_parent="$(dirname -- "$handoff_arg")"
mkdir -p -- "$handoff_parent"
handoff_dest="$(cd -- "$handoff_parent" && pwd)/$(basename -- "$handoff_arg")"

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
revision="$(git rev-parse HEAD)"
[[ "$(git rev-parse "refs/tags/$tag^{commit}")" == "$revision" ]] || { echo "prepare: $tag does not name the checked-out revision" >&2; exit 2; }
[[ -f .verify/receipt.json ]] || { echo "prepare: missing .verify/receipt.json from verify stage" >&2; exit 2; }

umask 077
work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT

tools/bootstrap/bazel.sh build --noshow_progress "--ui_event_filters=-info,-stdout,-stderr" \
  //bff/cmd/bff:image //bff/cmd/bff:image_tarball @bff_template_quality//tools:yq --output_groups=+tarball
parser="$(tools/bootstrap/bazel.sh cquery --output=files @bff_template_quality//tools:yq 2>/dev/null | head -1)"
layout="$(tools/bootstrap/bazel.sh cquery --output=files //bff/cmd/bff:image 2>/dev/null | head -1)"
tarball="$(tools/bootstrap/bazel.sh cquery --output=files --output_groups=tarball //bff/cmd/bff:image_tarball 2>/dev/null | tail -1)"

release_out="$(tools/checks/image_release.sh "$root/tools/bootstrap/bazel.sh" full "$parser")"
digest="$(sed -nE 's/^measurement: image (sha256:[0-9a-f]{64}),.*/\1/p' <<<"$release_out")"
[[ -n "$digest" ]] || { echo "prepare: the plain build's digest could not be measured" >&2; exit 2; }
[[ -d "$layout" && -f "$layout/index.json" ]] || { echo "prepare: plain OCI layout missing at $layout" >&2; exit 2; }
[[ -f "$tarball" ]] || { echo "prepare: release tarball missing at $tarball" >&2; exit 2; }

runtime_out="$(BFF_IMAGE_RUNTIME_OUTPUTS="$work/runtime" tools/checks/image_runtime.sh)"
runtime_digest="$(sed -nE 's/^measurement: runtime image (sha256:[0-9a-f]{64})$/\1/p' <<<"$runtime_out")"
[[ "$runtime_digest" == "$digest" ]] || { echo "prepare: actual-runtime digest does not match the plain built image" >&2; exit 1; }

staging="$work/handoff"
mkdir -p "$staging"
cp -- "$tarball" "$staging/bff-image-$tag.tar"
cp -- .verify/receipt.json "$staging/verify-receipt-$tag.json"
cp -- docs/provenance.md "$staging/provenance-$tag.md"
cp -- LICENSE "$staging/LICENSE"
cp -- docs/licenses/go-x509roots-BSD-3-Clause.txt "$staging/go-x509roots-BSD-3-Clause.txt"
tar -czf "$staging/runtime-acceptance-$tag.tar.gz" -C "$work/runtime" .
rm -rf -- "$work/runtime"
printf '%s\n' "$revision" >"$staging/source-revision-$tag.txt"
printf '%s\n' "$digest" >"$staging/image-digest-$tag.txt"

mapfile -t public_assets < <(python3 "$root/tools/release/verify_handoff.py" --list-public-assets --tag "$tag")
(cd "$staging" && LC_ALL=C sha256sum -- "${public_assets[@]}" >SHA256SUMS)
tar -czf "$staging/image-layout-$tag.tar.gz" -C "$layout" .
all_assets=("${public_assets[@]}" "SHA256SUMS" "image-layout-$tag.tar.gz")
(cd "$staging" && LC_ALL=C sha256sum -- "${all_assets[@]}" >HANDOFF.sha256)

verified="$(python3 "$root/tools/release/verify_handoff.py" --handoff "$staging" --tag "$tag" --revision "$revision")"
[[ "$verified" == "$digest" ]] || { echo "prepare: self-verification digest mismatch ($verified vs $digest)" >&2; exit 1; }

rm -rf -- "$handoff_dest"
mv "$staging" "$handoff_dest"
echo "prepare: verified $digest at $revision into $handoff_dest"
