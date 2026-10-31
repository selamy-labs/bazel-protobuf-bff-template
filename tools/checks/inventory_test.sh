#!/usr/bin/env bash
# Witnesses for input discovery (specs/002-hermetic-skeleton T025). The checkout path deliberately
# contains spaces and regular-expression metacharacters.
set -euo pipefail
inventory="$PWD/tools/checks/inventory.sh"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
failures=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }
has() { tr '\0' '\n' <"$1" | grep -qxF -- "$2"; }

repo="$work/x [1 (a|b)* \$HOME"
mkdir -p "$repo/tools/speckit" "$repo/.specify/scripts" "$repo/.specify/x" "$repo/.github/workflows" "$repo/sp ace"
git -C "$repo" init -q
sum="$(printf 'x' | sha256sum | cut -d' ' -f1)"
printf '%s  .specify/scripts/vendored.sh\n' "$sum" >"$repo/tools/speckit/scaffold.sha256"
echo x >"$repo/.specify/scripts/vendored.sh"
echo x >"$repo/.specify/x/evil.sh"
echo x >"$repo/sp ace/authored file.sh"
echo x >"$repo/MODULE.bazel"
echo x >"$repo/.github/workflows/ci.yml"
echo x >"$repo/gone.sh"
git -C "$repo" add -A
rm "$repo/gone.sh"
echo x >"$repo/untracked.sh"

out="$work/out" && mkdir -p "$out"
"$inventory" "$repo" "$out" || fail "valid repository under a hostile path was rejected"
has "$out/shell" "sp ace/authored file.sh" || fail "path with spaces was lost"
has "$out/shell" "untracked.sh" || fail "untracked file was lost"
has "$out/shell" ".specify/x/evil.sh" || fail "non-manifest file under .specify was excluded"
has "$out/shell" ".specify/scripts/vendored.sh" && fail "manifest-listed vendored file stayed in the authored set"
has "$out/all" ".specify/scripts/vendored.sh" || fail "vendored file must stay in the full inventory"
has "$out/deleted" "gone.sh" || fail "deleted file was not recorded"
has "$out/all" "gone.sh" && fail "deleted file stayed in the inventory"
grep -qx 'workflows=1' "$out/counts" || fail "counts were not recorded: $(tr '\n' ' ' <"$out/counts")"

# Shell and Starlark files are enrolled by content and name, not only by extension.
printf '#!/usr/bin/env bash\nexit 0\n' >"$repo/noextension"
printf '#!/bin/sh\nexit 0\n' >"$repo/script.bash"
printf 'not a script\n' >"$repo/notes.txt"
printf 'x = 1\n' >"$repo/BUILD"
mkdir -p "$repo/quality/checks/suppression/testdata"
printf '#!/bin/sh\n' >"$repo/quality/checks/suppression/testdata/S006_good.txt"
printf '%s  quality/checks/suppression/testdata/S006_good.txt\n' "$sum" >"$repo/quality/checks/suppression/testdata/fixtures.sha256"
ln -s missing "$repo/dangling"
"$inventory" "$repo" "$out" || fail "enrollment repository was rejected"
has "$out/shell" "noextension" || fail "extensionless bash script was not enrolled"
has "$out/shell" "script.bash" || fail ".bash script was not enrolled"
has "$out/starlark" "BUILD" || fail "plain BUILD file was not enrolled"
has "$out/shell" "notes.txt" && fail "text file was enrolled as shell"
has "$out/shell" "quality/checks/suppression/testdata/S006_good.txt" && fail "an enrolled fixture was treated as shell source"
has "$out/shell" "dangling" && fail "dangling symlink was treated as shell source"

# The tested tree is what was read from disk, whatever Git's index or local excludes say.
git -C "$repo" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -qm base
git -C "$repo" add -A
git -C "$repo" -c user.name=t -c user.email=t@example.invalid -c commit.gpgsign=false commit -qm all
"$inventory" "$repo" "$out" || fail "committed repository was rejected"
[[ "$(<"$out/tested_tree")" == "$(git -C "$repo" rev-parse 'HEAD^{tree}')" ]] || fail "clean tree must equal HEAD's tree"
clean_policy="$(<"$out/policy_digest")"
git -C "$repo" update-index --skip-worktree MODULE.bazel
echo changed >"$repo/MODULE.bazel"
"$inventory" "$repo" "$out" || fail "skip-worktree repository was rejected"
[[ "$(<"$out/tested_tree")" != "$(git -C "$repo" rev-parse 'HEAD^{tree}')" ]] || fail "a skip-worktree edit looked clean"
[[ "$(<"$out/policy_digest")" != "$clean_policy" ]] || fail "policy digest missed a MODULE.bazel edit"
git -C "$repo" update-index --no-skip-worktree MODULE.bazel
git -C "$repo" checkout -q -- MODULE.bazel
echo 'hidden.sh' >>"$repo/.git/info/exclude"
printf '#!/bin/sh\n' >"$repo/hidden.sh"
git -C "$repo" config status.showUntrackedFiles no
"$inventory" "$repo" "$out" || fail "excluded-file repository was rejected"
has "$out/shell" "hidden.sh" || fail ".git/info/exclude hid a file from the inventory"
[[ "$(<"$out/tested_tree")" != "$(git -C "$repo" rev-parse 'HEAD^{tree}')" ]] || fail "an excluded file looked clean"
rm "$repo/hidden.sh"
[[ -z "$(git -C "$repo" status --porcelain --untracked-files=no)" ]] || fail "the inventory changed the repository's status"

rejects() { # rejects <label> <needle> <root>
  local o rc=0
  o="$(mktemp -d "$work/o.XXXX")"
  "$inventory" "$3" "$o" >"$o/log" 2>&1 || rc=$?
  [[ $rc -ne 0 ]] || fail "$1: accepted"
  grep -qF -- "$2" "$o/log" || fail "$1: rejected for the wrong reason: $(head -1 "$o/log")"
}

rm -rf "${repo:?}/.github"
rejects "missing workflows" "category 'workflows'" "$repo"
mkdir -p "$repo/.github/workflows" && echo x >"$repo/.github/workflows/ci.yml"
rm "$repo/.specify/scripts/vendored.sh"
rejects "manifest entry missing from the tree" "manifest entry is missing" "$repo"
echo x >"$repo/.specify/scripts/vendored.sh"
: >"$repo/tools/speckit/scaffold.sha256"
rejects "empty manifest" "missing or empty" "$repo"
printf '%s  .specify/scripts/vendored.sh\n' "$sum" >"$repo/tools/speckit/scaffold.sha256"
rm -rf "$repo/.git"
rejects "not a Git repository" "fatal" "$repo"

[[ $failures -eq 0 ]] && echo "ok   inventory"
exit "$failures"
