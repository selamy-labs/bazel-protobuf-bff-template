#!/usr/bin/env bash
# Offline witness that the vendored Spec Kit scaffold matches its committed hash manifest.
# Fails on a modified, missing or extra scaffold file (specs/002-hermetic-skeleton FR-013).
set -euo pipefail
cd "${TEST_SRCDIR}/_main"
manifest="tools/speckit/scaffold.sha256"
status=0
if ! sha256sum --check --quiet --strict "$manifest"; then
  echo "speckit-scaffold: vendored file differs from $manifest (rerun: tools/speckit/bootstrap.sh verify)" >&2
  status=1
fi
listed="$(cut -c67- "$manifest" | LC_ALL=C sort)"
present="$(LC_ALL=C sort "${SCAFFOLD_LIST}")"
if [[ "$listed" != "$present" ]]; then
  echo "speckit-scaffold: scaffold file set differs from $manifest:" >&2
  diff <(echo "$listed") <(echo "$present") >&2 || true
  status=1
fi
exit "$status"
