#!/usr/bin/env bash
# Discovers the verifier's inputs (specs/002-hermetic-skeleton T025). All lists are NUL-delimited
# paths relative to <root>, so a checkout path can never be misread as a pattern. Vendored files
# are excluded by exact manifest entry, not by directory. Any discovery error or an unexpectedly
# empty category fails: a check given nothing must not report success.
#
# Local Git state cannot narrow the inventory: only committed .gitignore rules apply, not
# .git/info/exclude or any status setting. tested_tree is the Git tree of exactly what was found on
# disk, so comparing it with HEAD's tree shows whether the commit is what was tested.
#
#   tools/checks/inventory.sh <root> <outdir>
set -euo pipefail
root="$1"
out="$2"
manifest="tools/speckit/scaffold.sha256"
fixtures="quality/checks/suppression/testdata/fixtures.sha256"
cd "$root"

die() { echo "inventory: $1 (rerun: tools/verify.sh)" >&2; exit 1; }

# grep exits 1 for "no lines selected", which is a legitimate empty result; 2 is a real error.
select_lines() {
  local rc=0
  grep "$@" || rc=$?
  [[ $rc -le 1 ]] || die "file selection failed: grep $*"
}

[[ -s "$manifest" ]] || die "$manifest is missing or empty"
git ls-files -z --cached --others --exclude-per-directory=.gitignore >"$out/listed"
sort -zu "$out/listed" >"$out/tracked"

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
tr '\0' '\n' <"$out/authored" >"$out/authored.nl"

# Enrolled defect fixtures are inputs to the scanner's tests, not source for other checkers.
if [[ -f "$fixtures" ]]; then cut -c67- "$fixtures" >"$out/fixtures.nl"; else : >"$out/fixtures.nl"; fi
: >"$out/shell"
while IFS= read -r -d '' path; do
  if grep -qFx -- "$path" "$out/fixtures.nl" || [[ ! -f "$path" ]]; then continue; fi
  first=""
  case "$path" in
    *.sh | *.bash) printf '%s\0' "$path" >>"$out/shell" ;;
    *)
      IFS= read -r first <"$path" 2>/dev/null || true
      if [[ "$first" =~ ^\#!.*[/\ ](bash|dash|ksh|zsh|sh)([[:space:]]|$) ]]; then
        printf '%s\0' "$path" >>"$out/shell"
      fi
      ;;
  esac
done <"$out/authored"
select_lines -zE '(^|/)(BUILD|BUILD\.bazel|MODULE\.bazel|REPO\.bazel)$|\.bzl$' "$out/authored" >"$out/starlark"
select_lines -zE '^\.github/workflows/[^/]+\.ya?ml$' "$out/authored" >"$out/workflows"

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

# Objects go to a scratch directory, so measuring the tree writes nothing into the repository.
mkdir -p "$out/objects"
objects="$(cd "$(git rev-parse --git-path objects)" && pwd)"
export GIT_INDEX_FILE="$out/index" GIT_OBJECT_DIRECTORY="$out/objects" GIT_ALTERNATE_OBJECT_DIRECTORIES="$objects"
git -c core.fsmonitor=false update-index --add -z --stdin <"$out/all"
git write-tree >"$out/tested_tree"
git ls-tree -r "$(<"$out/tested_tree")" -- .bazelrc .bazelversion BUILD.bazel MODULE.bazel MODULE.bazel.lock \
  REPO.bazel .gitleaks.toml _typos.toml quality tools .github | sha256sum | cut -d' ' -f1 >"$out/policy_digest"
