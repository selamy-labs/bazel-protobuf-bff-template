#!/usr/bin/env bash
# Discovers the verifier's inputs (specs/002-hermetic-skeleton T025). All lists are NUL-delimited
# paths relative to <root>, so a checkout path can never be misread as a pattern. Vendored files
# are excluded by exact manifest entry, not by directory. Any discovery error or an unexpectedly
# empty category fails: a check given nothing must not report success.
#
#   tools/checks/inventory.sh <root> <outdir>
set -euo pipefail
root="$1"
out="$2"
manifest="tools/speckit/scaffold.sha256"
cd "$root"

die() { echo "inventory: $1 (rerun: tools/verify.sh)" >&2; exit 1; }

# grep exits 1 for "no lines selected", which is a legitimate empty result; 2 is a real error.
select_lines() {
  local rc=0
  grep "$@" || rc=$?
  [[ $rc -le 1 ]] || die "file selection failed: grep $*"
}

[[ -s "$manifest" ]] || die "$manifest is missing or empty"
git ls-files -z --cached --others --exclude-standard >"$out/tracked"

: >"$out/all"
: >"$out/deleted"
while IFS= read -r -d '' path; do
  if [[ -e "$path" || -L "$path" ]]; then
    printf '%s\0' "$path" >>"$out/all"
  else
    printf '%s\0' "$path" >>"$out/deleted"
  fi
done <"$out/tracked"

cut -c67- "$manifest" >"$out/vendored.nl"
select_lines -zvFxf "$out/vendored.nl" "$out/all" >"$out/authored"
select_lines -zE '\.sh$' "$out/authored" >"$out/shell"
select_lines -zE '(^|/)(BUILD\.bazel|MODULE\.bazel|REPO\.bazel)$|\.bzl$' "$out/authored" >"$out/starlark"
select_lines -zE '^\.github/workflows/[^/]+\.ya?ml$' "$out/authored" >"$out/workflows"
tr '\0' '\n' <"$out/authored" >"$out/authored.nl"

count() { tr -cd '\0' <"$1" | wc -c | tr -d ' '; }
for category in all authored shell starlark workflows; do
  [[ "$(count "$out/$category")" -gt 0 ]] || die "no files in category '$category'; discovery is broken or enrollment was narrowed"
done
# A vendored entry that is absent would silently shrink the scanned set.
while IFS= read -r path; do
  [[ -f "$path" ]] || die "manifest entry is missing from the tree: $path"
done <"$out/vendored.nl"

for category in all authored deleted shell starlark workflows; do
  echo "$category=$(count "$out/$category")"
done >"$out/counts"
