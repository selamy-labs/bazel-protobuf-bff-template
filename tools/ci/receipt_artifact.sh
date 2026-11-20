#!/usr/bin/env bash
# Prepare only this receipt's owned bounded files; this command does not judge or change its verdict.
set -euo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$root"
exec "$root/tools/bootstrap/bazel.sh" run --noshow_progress //:quality -- "--export-receipt=$root/.verify/receipt.json"
