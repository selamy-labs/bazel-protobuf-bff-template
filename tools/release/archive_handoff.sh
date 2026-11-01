#!/usr/bin/env bash
# The lifecycle owner's caller of the archive primitive (spec 011 FR-015). It records cleanup eligibility only
# behind a verified archive: the primitive must succeed and report the bundle's sha256 and the exact commit and
# tree, and the record names them. A failed archive leaves no record, so nothing downstream can read
# eligibility. This script performs no cleanup; custody of any cleanup stays with the owner who reads the record.
#
#   archive_handoff.sh <repository> <refs/...> <record directory>
set -euo pipefail
umask 077
repo="$1"; ref="$2"; dir="$3"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
[[ -d "$dir" && ! -L "$dir" ]] || { echo "handoff: the record directory must exist and not be a link" >&2; exit 2; }
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
bundle="$dir/$stamp.bundle"
if ! report="$("$here/archive_ref.sh" "$repo" "$ref" "$bundle" 2>"$dir/$stamp.failed")"; then
  echo "handoff: the archive failed; no eligibility record exists, and cleanup stays ineligible" >&2
  exit 2
fi
rm -f "$dir/$stamp.failed"
commit="$(sed -nE 's/^archive: .* at ([0-9a-f]{40}), tree .*/\1/p' <<<"$report")"
tree="$(sed -nE 's/^archive: .* tree ([0-9a-f]{40}), .*/\1/p' <<<"$report")"
sum="$(sed -nE 's/^archive: .* bundle sha256 ([0-9a-f]{64}),.*/\1/p' <<<"$report")"
[[ -n "$commit" && -n "$tree" && -n "$sum" ]] || { echo "handoff: the archive report is incomplete; no record" >&2; exit 2; }
printf '{"ref":"%s","commit":"%s","tree":"%s","bundle":"%s","bundle_sha256":"%s","archived_utc":"%s","cleanup":"not authorized by this record; the owner decides, citing the bundle sha256 and commit"}\n' "$ref" "$commit" "$tree" "$bundle" "$sum" "$stamp" >"$dir/$stamp.eligibility.json"
echo "handoff: archived $ref at $commit; record $dir/$stamp.eligibility.json"
