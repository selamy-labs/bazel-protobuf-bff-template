#!/usr/bin/env bash
# Write-scoped Artifact Registry publisher: fetches a generation-pinned immutable release handoff
# archive from GCS, verifies its SHA-256, handoff integrity, OCI/tarball closure, terminal
# actual-container runtime acceptance, and full verify receipt BEFORE registry auth or crane push.
set -euo pipefail

: "${RELEASE_COMMIT:?}" "${RELEASE_TAG:?}" "${RELEASE_EXPECTED_DIGEST:?}" \
  "${RELEASE_HANDOFF_URI:?}" "${RELEASE_HANDOFF_SHA256:?}" "${RELEASE_BUILD_ID:?}" \
  "${RELEASE_PROJECT:?}" "${RELEASE_REGION:?}" "${RELEASE_REPOSITORY:?}"

[[ "$RELEASE_COMMIT" =~ ^[0-9a-f]{40}$ ]] || {
  echo "publish-image: invalid RELEASE_COMMIT" >&2
  exit 1
}
[[ "$RELEASE_TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || {
  echo "publish-image: invalid RELEASE_TAG" >&2
  exit 1
}
[[ "$RELEASE_EXPECTED_DIGEST" =~ ^sha256:[0-9a-f]{64}$ ]] || {
  echo "publish-image: invalid RELEASE_EXPECTED_DIGEST" >&2
  exit 1
}
[[ "$RELEASE_HANDOFF_URI" =~ ^gs://[a-z0-9][a-z0-9._-]+/[A-Za-z0-9._/-]+#[1-9][0-9]*$ ]] || {
  echo "publish-image: RELEASE_HANDOFF_URI must be a generation-pinned gs:// object" >&2
  exit 1
}
[[ "/$RELEASE_HANDOFF_URI/" != *"/../"* ]] || {
  echo "publish-image: RELEASE_HANDOFF_URI must not contain path traversal" >&2
  exit 1
}
[[ "$RELEASE_HANDOFF_SHA256" =~ ^[0-9a-f]{64}$ ]] || {
  echo "publish-image: invalid RELEASE_HANDOFF_SHA256" >&2
  exit 1
}
[[ "$RELEASE_BUILD_ID" =~ ^[A-Za-z0-9._-]+$ ]] || {
  echo "publish-image: invalid RELEASE_BUILD_ID" >&2
  exit 1
}
[[ "$RELEASE_PROJECT" =~ ^[a-z][a-z0-9-]{4,28}[a-z0-9]$ ]] || {
  echo "publish-image: invalid RELEASE_PROJECT" >&2
  exit 1
}
[[ "$RELEASE_REGION" =~ ^[a-z]+-[a-z]+[0-9]+$ ]] || {
  echo "publish-image: invalid RELEASE_REGION" >&2
  exit 1
}
[[ "$RELEASE_REPOSITORY" =~ ^[a-z][a-z0-9-]{0,61}[a-z0-9]$ ]] || {
  echo "publish-image: invalid RELEASE_REPOSITORY" >&2
  exit 1
}

: "${HOME:?Cloud Build must provide its runtime home}"
[[ ! -e "$HOME/.config/gcloud/application_default_credentials.json" ]] || {
  echo "publish-image: well-known ADC file is not allowed in the publisher" >&2
  exit 1
}

command -v gcloud >/dev/null || {
  echo "publish-image: gcloud is required to fetch the immutable handoff archive" >&2
  exit 1
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd -- "$script_dir/../../.." && pwd)"
verify_handoff="${RELEASE_VERIFY_HANDOFF_SCRIPT:-$root/tools/release/verify_handoff.py}"
verify_runtime="$script_dir/verify_runtime_evidence.py"
[[ -f "$verify_handoff" && ! -L "$verify_handoff" ]] || {
  echo "publish-image: verify_handoff.py not found at $verify_handoff" >&2
  exit 1
}
[[ -f "$verify_runtime" && ! -L "$verify_runtime" ]] || {
  echo "publish-image: verify_runtime_evidence.py not found at $verify_runtime" >&2
  exit 1
}

work="$(mktemp -d "${TMPDIR:-/tmp}/bff-publish.XXXXXXXX")"
trap 'rm -rf -- "$work"' EXIT
archive="$work/handoff.tar.gz"
handoff_dir="$work/handoff"
layout="$work/layout"

# 1. Fetch the generation-pinned immutable handoff archive, verify its exact SHA-256, and unpack safely.
gcloud storage cp -- "$RELEASE_HANDOFF_URI" "$archive"
env -u GITHUB_TOKEN -u GH_TOKEN python3 -B "$verify_runtime" extract-transport \
  --archive "$archive" \
  --expected-sha256 "$RELEASE_HANDOFF_SHA256" \
  --dest "$handoff_dir" \
  --verify-handoff "$verify_handoff"

# 2. Verify complete handoff integrity, revision, OCI closure, and Docker tarball layer binding.
verified_digest="$(env -u GITHUB_TOKEN -u GH_TOKEN python3 -B "$verify_handoff" \
  --handoff "$handoff_dir" \
  --tag "$RELEASE_TAG" \
  --revision "$RELEASE_COMMIT" \
  --extract-layout "$layout")"

[[ "$verified_digest" == "$RELEASE_EXPECTED_DIGEST" ]] || {
  echo "publish-image: handoff digest mismatch ($verified_digest != $RELEASE_EXPECTED_DIGEST)" >&2
  exit 1
}

# 3. Verify terminal actual-container runtime acceptance (exit 0, Playwright report, identity) and
# full clean verify receipt BEFORE registry authentication or crane push.
env -u GITHUB_TOKEN -u GH_TOKEN python3 -B "$verify_runtime" verify-evidence \
  --handoff "$handoff_dir" \
  --tag "$RELEASE_TAG" \
  --revision "$RELEASE_COMMIT" \
  --expected-digest "$verified_digest" \
  --layout "$layout" \
  --verify-handoff "$verify_handoff"

command -v docker-credential-gcr >/dev/null || {
  echo "publish-image: standalone Artifact Registry credential helper is required" >&2
  exit 1
}
command -v crane >/dev/null || {
  echo "publish-image: crane is required to publish OCI layouts by digest" >&2
  exit 1
}

export DOCKER_CONFIG="$work/registry-auth"
mkdir -p "$DOCKER_CONFIG"
export DOCKER_CREDENTIAL_GCR_STORE="$DOCKER_CONFIG/empty-store.json"
export DOCKER_CREDENTIAL_GCR_CONFIG="$DOCKER_CONFIG/helper-config.json"
printf '%s\n' '{"TokenSources":["env"]}' > "$DOCKER_CREDENTIAL_GCR_CONFIG"
unset GOOGLE_APPLICATION_CREDENTIALS
unset GCE_METADATA_HOST GCE_METADATA_IP

python3 -B - "$RELEASE_REGION" "$DOCKER_CONFIG/config.json" <<'PY'
import json
import sys

with open(sys.argv[2], "w", encoding="utf-8") as handle:
    json.dump({"credHelpers": {f"{sys.argv[1]}-docker.pkg.dev": "gcr"}}, handle)
PY

image="${RELEASE_REGION}-docker.pkg.dev/${RELEASE_PROJECT}/${RELEASE_REPOSITORY}/bff"
pushed_ref="${image}:${RELEASE_COMMIT}"
crane push "$layout" "$pushed_ref"
remote_digest="$(crane digest "$pushed_ref")"
[[ "$remote_digest" == "$verified_digest" ]] || {
  echo "publish-image: published digest mismatch ($remote_digest != $verified_digest)" >&2
  exit 1
}

receipt="$(python3 -B - "$RELEASE_COMMIT" "$RELEASE_TAG" "$RELEASE_BUILD_ID" "$image" "$remote_digest" "$pushed_ref" "$RELEASE_HANDOFF_URI" "$RELEASE_HANDOFF_SHA256" <<'PY'
import json
import sys

print(
    json.dumps(
        {
            "source_revision": sys.argv[1],
            "tag": sys.argv[2],
            "cloud_build_id": sys.argv[3],
            "image": sys.argv[4],
            "digest": sys.argv[5],
            "pushed_ref": sys.argv[6],
            "handoff_uri": sys.argv[7],
            "handoff_sha256": sys.argv[8],
            "runtime_witness_verified": True,
        },
        sort_keys=True,
    )
)
PY
)"

printf '%s\n' "$receipt"
if [[ -n "${RELEASE_RECEIPT_PATH:-}" ]]; then
  printf '%s\n' "$receipt" > "$RELEASE_RECEIPT_PATH"
fi
