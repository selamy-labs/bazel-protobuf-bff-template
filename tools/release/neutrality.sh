#!/usr/bin/env bash
# The release scans (spec 010 FR-005 to FR-007). Both read the whole export of one repository: every blob and
# path reachable from any ref, every commit message, author and committer, every ref name and tag message.
#
#   neutrality.sh <repository> [--allow <file>] [--markers <file> --mapping <file>]
#
# Nothing matched is ever printed. A generic hit prints its category and a locator that is a Git object id
# or a count, never a path or the matched text. A private hit prints a run-local label and counts by scope;
# the locators go to --mapping, a private file this script creates or verifies at mode 0600, outside the
# repository and never through a link. With no --markers, or a file with no terms, the private status is
# not-run, which is never a pass. A marker file or blob that cannot be read is an error, not a result.
#
# Exit 0 when the generic scan passed and the private scan passed or did not run; 1 on any hit; 2 cannot read.
set -euo pipefail
repo="${1:?usage: neutrality.sh <repository> [--allow <file>] [--markers <file> --mapping <file>]}"
shift
allow="" markers="" mapping=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --allow) allow="$2"; shift 2 ;;
    --markers) markers="$2"; shift 2 ;;
    --mapping) mapping="$2"; shift 2 ;;
    *) echo "neutrality: unknown argument" >&2; exit 2 ;;
  esac
done
die() { echo "neutrality: $1" >&2; exit 2; }
git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || die "the first argument is not a repository"
repo_top="$(git -C "$repo" rev-parse --path-format=absolute --show-toplevel 2>/dev/null || git -C "$repo" rev-parse --path-format=absolute --git-common-dir)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Every term and marker is read into files, never into pipelines whose exit a match could cut short.
git -C "$repo" rev-list --all --reflog >"$work/commits.list" || die "cannot list commits"
# Every path association in every reachable tree, raw: ls-tree -z keeps names literal, and nothing is deduplicated
# before a name is seen. rev-list --objects would show one name for identical content under two names.
: >"$work/paths.z"
while read -r commit; do
  git -C "$repo" ls-tree -r -z --format='%(objectname)%x09%(path)%x00' "$commit" >>"$work/paths.z" || die "cannot list the tree of $commit"
done <"$work/commits.list"
LC_ALL=C sort -z -u "$work/paths.z" >"$work/paths.zu"
cut -z -f1 "$work/paths.zu" | tr '\0' '\n' | sort -u >"$work/blobs.candidates"
git -C "$repo" cat-file --batch-check='%(objectname) %(objecttype)' <"$work/blobs.candidates" | awk '$2 == "blob" {print $1}' | sort -u >"$work/blobs" || die "cannot type objects"
cut -z -f2- "$work/paths.zu" | LC_ALL=C sort -z -u >"$work/pathnames.z"
git -C "$repo" log --all --reflog --format='%H%x1f%an%x1f%ae%x1f%cn%x1f%ce%x1f%B%x1e' >"$work/commits" || die "cannot read commits"
git -C "$repo" for-each-ref --format='%(refname)%x1f%(contents)' >"$work/refs" || die "cannot read refs"
commits="$(wc -l <"$work/commits.list")"
blobs="$(wc -l <"$work/blobs")"
paths="$(tr -cd '\0' <"$work/pathnames.z" | wc -c)"
scope="$commits commits, $blobs blobs, $paths distinct paths from every reachable tree, $(wc -l <"$work/refs") refs, all identities"
mkdir "$work/blob"
while read -r sha; do git -C "$repo" cat-file -p "$sha" >"$work/blob/$sha" || die "cannot read blob $sha"; done <"$work/blobs"
tr '\0' '\n' <"$work/pathnames.z" >"$work/pathnames"
tr '\037' '\n' <"$work/refs" >"$work/refnames"
mkdir "$work/commit"
while IFS= read -r -d $'\x1e' record; do
  sha="${record%%$'\x1f'*}"
  printf '%s' "${record#*$'\x1f'}" | tr '\037' '\n' >"$work/commit/$sha"
done <"$work/commits"

# Generic. grep reads a file to its end, so a match and a read error are distinct.
: >"$work/hits"
hit() { echo "neutrality: generic $1 at $2" | tee -a "$work/hits"; }
scan_file() { # scan_file <file> <locator>
  local file="$1" locator="$2"
  grep -aqE -- '-----BEGIN [A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{36}|xox[baprs]-[0-9A-Za-z-]{10,}' "$file" && hit "secret pattern" "$locator"
  grep -aqE -- '[a-z0-9-]+\.iam\.gserviceaccount\.com|projects/[0-9]{9,}|arn:aws:[a-z0-9-]+:' "$file" && hit "cloud identifier" "$locator"
  grep -aqE -- '/home/[A-Za-z0-9_-]+/|/Users/[A-Za-z0-9_-]+/|C:[\\]Users[\\]' "$file" && hit "personal path" "$locator"
  grep -aoE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' "$file" >"$work/addresses" || [[ $? -eq 1 ]] || die "cannot read $locator"
  sed 's/.*@//' "$work/addresses" | tr '[:upper:]' '[:lower:]' | sort -u >"$work/domains"
  local outside=0 domain
  while read -r domain; do
    [[ -z "$domain" ]] && continue
    if [[ -n "$allow" ]] && grep -qxF -- "$domain" "$allow"; then continue; fi
    outside=$((outside + 1))
  done <"$work/domains"
  [[ $outside -gt 0 ]] && hit "$outside email domains outside the allowlist" "$locator"
  return 0
}
while read -r sha; do scan_file "$work/blob/$sha" "blob $sha"; done <"$work/blobs"
scan_file "$work/pathnames" "a path"
for file in "$work"/commit/*; do [[ -e "$file" ]] || continue; scan_file "$file" "commit ${file##*/}"; done
scan_file "$work/refnames" "a ref name or tag message"
hits="$(wc -l <"$work/hits")"
if [[ $hits -eq 0 ]]; then echo "neutrality: generic passed, scope: $scope"; else echo "neutrality: generic failed, $hits hits, scope: $scope"; fi

# Private: labels only. A term list with no terms is not-run; an unreadable list is an error.
private=0
if [[ -z "$markers" ]]; then
  echo "neutrality: private not-run (no marker file), scope: $scope"
else
  [[ -r "$markers" ]] || die "the marker file cannot be read"
  terms=()
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line#"${line%%[![:space:]]*}"}"; line="${line%"${line##*[![:space:]]}"}"
    [[ -n "$line" ]] && terms+=("$line")
  done <"$markers"
  if [[ ${#terms[@]} -eq 0 ]]; then
    echo "neutrality: private not-run (the marker file holds no terms), scope: $scope"
  else
    [[ -n "$mapping" ]] || die "--markers needs --mapping"
    mapping_dir="$(cd "$(dirname "$mapping")" 2>/dev/null && pwd -P)" || die "the mapping file's directory does not exist"
    [[ "$mapping_dir" != "$repo_top" && "$mapping_dir" != "$repo_top"/* ]] || die "the mapping file must be outside the repository"
    [[ ! -L "$mapping" && ! -e "$mapping" ]] || die "the mapping file must be a new path: it exists, or it is a link"
    [[ "$mapping_dir/$(basename "$mapping")" != "$(realpath -e "$markers")" ]] || die "the mapping file must not be the marker file"
    (umask 077 && set -o noclobber && : >"$mapping") || die "cannot create the mapping file exclusively"
    [[ ! -L "$mapping" && -f "$mapping" && "$(stat -c %a "$mapping")" == 600 ]] || die "the mapping file is not a private regular file"
    n=0
    for term in "${terms[@]}"; do
      n=$((n + 1)); label="M$n"
      printf '%s' "$term" >"$work/term"
      cb=0; cp=0; cm=0; cr=0
      while read -r sha; do
        if grep -aqiF -f "$work/term" -- "$work/blob/$sha"; then cb=$((cb + 1)); echo "$label blob $sha" >>"$mapping"; fi
      done <"$work/blobs"
      while IFS= read -r -d '' entry; do
        sha="${entry%%$'\t'*}"; path="${entry#*$'\t'}"
        if printf '%s' "$path" | grep -aqiF -f "$work/term" --; then cp=$((cp + 1)); echo "$label path $sha" >>"$mapping"; fi
      done <"$work/paths.zu"
      for file in "$work"/commit/*; do
        [[ -e "$file" ]] || continue
        if grep -aqiF -f "$work/term" -- "$file"; then cm=$((cm + 1)); echo "$label commit ${file##*/}" >>"$mapping"; fi
      done
      while IFS=$'\x1f' read -r ref contents; do
        if grep -qiF -f "$work/term" -- <<<"$ref$contents"; then cr=$((cr + 1)); echo "$label ref $(printf '%s' "$ref" | sha256sum | cut -c1-16)" >>"$mapping"; fi
      done <"$work/refs"
      total=$((cb + cp + cm + cr))
      if [[ $total -gt 0 ]]; then
        echo "neutrality: private $label: $total hits (blobs $cb, paths $cp, commits $cm, refs $cr)"
        private=$((private + total))
      fi
    done
    if [[ $private -eq 0 ]]; then echo "neutrality: private passed, ${#terms[@]} markers literally absent, scope: $scope"; else echo "neutrality: private failed, $private hits over ${#terms[@]} markers, locators in the mapping file only, scope: $scope"; fi
  fi
fi
[[ $hits -eq 0 && $private -eq 0 ]]
