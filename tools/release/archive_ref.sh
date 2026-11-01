#!/usr/bin/env bash
# A bounded archive primitive (spec 011 FR-015), for a maintained lifecycle owner: one named ref of a repository is
# bundled into an exclusively owned temporary directory, the bundle's header is verified against the source, and a
# fresh isolated clone of the bundle alone, which is what reads the pack, must resolve the same commit and tree.
# `git bundle verify` alone proves nothing about the pack data, so the restore is the proof. The bundle is then
# published without clobbering, at mode 0600. Nothing outside the owned directory is ever removed, nothing in the
# source repository changes, and no cleanup exists here: a valid archive is not an authorization to delete
# anything, and custody of any cleanup stays with the lifecycle owner.
#
#   archive_ref.sh <repository> <refs/...> <bundle path, new>
#
# Exit 0 with the bundle's sha256 and the resolved commit and tree; 2 on any refusal or failure, with no bundle.
set -euo pipefail
umask 077
repo="${1:?usage: archive_ref.sh <repository> <refs/...> <bundle path>}"
ref="${2:?usage: archive_ref.sh <repository> <refs/...> <bundle path>}"
bundle="${3:?usage: archive_ref.sh <repository> <refs/...> <bundle path>}"
own=""
die() { echo "archive: $1" >&2; [[ -n "$own" ]] && rm -rf "$own"; exit 2; }
[[ "$ref" == refs/* ]] || die "the archive takes a named ref under refs/, not an object id or a short name"
git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || die "not a repository"
commit="$(git -C "$repo" rev-parse --verify --quiet "$ref^{commit}")" || die "$ref does not name a commit"
tree="$(git -C "$repo" rev-parse --verify --quiet "$commit^{tree}")" || die "$commit has no tree"
[[ ! -e "$bundle" && ! -L "$bundle" ]] || die "the bundle path exists or is a link; the archive never overwrites"
dir="$(dirname "$bundle")"; [[ -d "$dir" && ! -L "$dir" ]] || die "the bundle's directory must exist and not be a link"
own="$(mktemp -d "$dir/.archive.XXXXXXXX")" || die "cannot create an owned directory beside the bundle"
git -C "$repo" bundle create --quiet "$own/bundle" "$ref" || die "bundle creation failed"
git -C "$repo" bundle verify --quiet "$own/bundle" || die "the bundle's header does not verify against the source"
git clone --quiet --bare "$own/bundle" "$own/restore" 2>/dev/null || die "a fresh clone of the bundle failed"
restored="$(git -C "$own/restore" rev-parse --verify --quiet "$ref^{commit}")" || die "the restored clone does not resolve $ref"
[[ "$restored" == "$commit" ]] || die "the restored commit $restored is not $commit"
git -C "$own/restore" cat-file -e "$commit" || die "the restored clone lacks the commit object"
git -C "$own/restore" cat-file -e "$tree" || die "the restored clone lacks the tree object"
[[ "$(git -C "$own/restore" rev-parse "$commit^{tree}")" == "$tree" ]] || die "the restored tree differs"
chmod 0600 "$own/bundle" || die "cannot set the bundle's mode"
# The digest and mode are read inside the owned directory, before publication, and a read failure is a failure:
# nothing reported below comes from the destination path, which another writer could have changed meanwhile.
sum="$(sha256sum "$own/bundle" | cut -d' ' -f1)" || die "cannot read the bundle's digest"
[[ "$sum" =~ ^[0-9a-f]{64}$ ]] || die "the bundle's digest is not a sha256"
mode="$(stat -c %a "$own/bundle")" || die "cannot read the bundle's mode"
[[ "$mode" == 600 ]] || die "the bundle's mode is $mode, not 600"
# -T names the destination itself: a directory or a link that arrived at the bundle path since the first check is
# refused as an existing path, never entered or dereferenced. The published path must then be the owned bundle.
ln -T "$own/bundle" "$bundle" 2>/dev/null || die "the bundle path appeared meanwhile; nothing was written there"
[[ ! -L "$bundle" && "$bundle" -ef "$own/bundle" ]] || die "the published path is not the owned bundle; nothing is reported as archived"
rm -rf "$own"; own=""
echo "archive: $ref at $commit, tree $tree, bundle sha256 $sum, mode $mode"
