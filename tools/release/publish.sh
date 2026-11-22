#!/usr/bin/env bash
# Trusted publishing consumer (spec 010 FR-013, spec 009 FR-016): verifies the credentialless
# release handoff artifact integrity, revision binding, OCI closure, and tarball identity BEFORE
# logging in to the destination registry or creating the release. Pushes only via maintained pinned
# ORAS by verified digest without rebuilding or running browser/Docker verification under credentials.
#
#   tools/release/publish.sh <owner/repository> <tag> <handoff-dir>
set -euo pipefail
[[ $# -eq 3 ]] || { echo "publish: usage: publish.sh <owner/repository> <tag> <handoff-dir>" >&2; exit 2; }
repository="$1"; tag="$2"; handoff_arg="$3"
[[ -n "${GITHUB_TOKEN:-}" ]] || { echo "publish: GITHUB_TOKEN is not set; this runs only in the publishing job" >&2; exit 2; }
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || { echo "publish: $tag is not a release tag" >&2; exit 2; }
[[ -d "$handoff_arg" && ! -L "$handoff_arg" ]] || { echo "publish: handoff directory not found: $handoff_arg" >&2; exit 2; }
handoff="$(cd -- "$handoff_arg" && pwd)"

root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
revision="$(git rev-parse HEAD)"
[[ "$(git rev-parse "refs/tags/$tag^{commit}")" == "$revision" ]] || { echo "publish: $tag does not name the checked-out revision" >&2; exit 2; }

umask 077
work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT

digest="$(env -u GITHUB_TOKEN -u GH_TOKEN python3 "$root/tools/release/verify_handoff.py" \
  --handoff "$handoff" --tag "$tag" --revision "$revision" --extract-layout "$work/layout")"
[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "publish: handoff returned invalid digest" >&2; exit 1; }

registry="ghcr.io"
package="$registry/${repository,,}/bff"
mkdir -p "$work/registry"
registry_config="$work/registry/config.json"

printf '%s' "$GITHUB_TOKEN" | env -u GITHUB_TOKEN -u GH_TOKEN "$root/tools/release/oras.sh" login \
  --registry-config "$registry_config" -u "${GITHUB_ACTOR:-token}" --password-stdin "$registry" >/dev/null
env -u GITHUB_TOKEN -u GH_TOKEN "$root/tools/release/oras.sh" cp --from-oci-layout --no-tty \
  --to-registry-config "$registry_config" "$work/layout@$digest" "$package:$tag"
pushed="$(env -u GITHUB_TOKEN -u GH_TOKEN "$root/tools/release/oras.sh" resolve \
  --registry-config "$registry_config" "$package:$tag" | tr -d '[:space:]')"
[[ "$pushed" == "$digest" ]] || { echo "publish: the registry resolved $package:$tag to '${pushed:-empty}', want $digest" >&2; exit 1; }

assets=()
while IFS= read -r rel; do
  rel="${rel#./}"
  [[ -n "$rel" && -f "$handoff/$rel" && ! -L "$handoff/$rel" ]] || { echo "publish: missing verified asset $rel" >&2; exit 1; }
  assets+=("$handoff/$rel")
done < <(awk '{print $2}' "$handoff/SHA256SUMS")
assets+=("$handoff/SHA256SUMS")

gh release create "$tag" --repo "$repository" --verify-tag --prerelease --title "$tag" \
  --notes "Pre-release $tag at $revision. Image $package@$digest. Not a stable release; the quality baseline is open (spec 011)." \
  "${assets[@]}"
echo "publish: $package@$digest and the assets of $tag at $revision"
