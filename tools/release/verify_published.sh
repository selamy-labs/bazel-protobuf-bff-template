#!/usr/bin/env bash
# The credentialless check of a publication (spec 010 FR-014): pull the image by digest anonymously, which only
# a public package allows, and download the release assets anonymously and check their hashes. It carries no
# token; a declared workflow is not live proof, this is.
#
#   tools/release/verify_published.sh <owner/repository> <tag>
set -euo pipefail
repository="$1"; tag="$2"
[[ -z "${GITHUB_TOKEN:-}" && -z "${GH_TOKEN:-}" ]] || { echo "verify_published: runs without a token, by design" >&2; exit 2; }
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
api="https://api.github.com/repos/$repository/releases/tags/$tag"
curl -sSf --max-time 60 "$api" >"$work/release.json"
mapfile -t urls < <(sed -nE 's/.*"browser_download_url": *"([^"]+)".*/\1/p' "$work/release.json")
[[ ${#urls[@]} -ge 2 ]] || { echo "verify_published: the release $tag lists ${#urls[@]} assets" >&2; exit 1; }
mkdir -p "$work/assets"
for url in "${urls[@]}"; do curl -sSfL --max-time 600 -o "$work/assets/$(basename "$url")" "$url"; done
(cd "$work/assets" && sha256sum -c SHA256SUMS --quiet) || { echo "verify_published: an asset's hash does not match SHA256SUMS" >&2; exit 1; }
digest="$(cat "$work/assets/image-digest-$tag.txt")"
[[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "verify_published: no image digest among the assets" >&2; exit 1; }
package="ghcr.io/${repository,,}/bff"
docker logout ghcr.io >/dev/null 2>&1 || true
docker pull --quiet "$package@$digest" >/dev/null || { echo "verify_published: an anonymous pull of $package@$digest failed; the package is not public or the digest is not there" >&2; exit 1; }
served="$(docker image inspect "$package@$digest" --format '{{index .RepoDigests 0}}')"
[[ "$served" == "$package@$digest" ]] || { echo "verify_published: pulled $served, not $package@$digest" >&2; exit 1; }
echo "verify_published: $package@$digest pulled anonymously; ${#urls[@]} assets of $tag match SHA256SUMS"
