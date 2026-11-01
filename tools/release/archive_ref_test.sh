#!/usr/bin/env bash
# Controls for archive_ref.sh (spec 011 SC-005): a bare object id is refused and leaves no bundle; a named ref is
# bundled, verified and restored to the same commit and tree; a damaged bundle fails the fresh restore; a directory
# or a link arriving at the destination after the restore is refused with nothing written and no archive reported;
# the source repository is byte-identical after every case. No cleanup is ever run.
set -euo pipefail
S="$PWD/$1"; T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
export GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL=a@example.com GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@example.com
git init -q -b main "$T/r"
(cd "$T/r" && printf 'one\n' >a.txt && git add . && git commit -q -m one && printf 'two\n' >b.txt && git add . && git commit -q -m two)
commit="$(git -C "$T/r" rev-parse HEAD)"; tree="$(git -C "$T/r" rev-parse 'HEAD^{tree}')"
fingerprint() { (cd "$T/r/.git" && find . -type f -exec sha256sum {} + | sort | sha256sum | cut -c1-16); }
before="$(fingerprint)"
failures=0
ok() { echo "ok   $1"; }
fail() { echo "FAIL $1"; failures=$((failures + 1)); }
# check <name> <command...>: the command's success is the control
check() { local name="$1"; shift; if "$@"; then ok "$name"; else fail "$name"; fi; }
# expect <name> <exit> <archive arguments...>: the archive's exit code is the control; its output is in $T/out
expect() {
  local name="$1" want="$2" rc=0; shift 2
  "$S" "$@" >"$T/out" 2>&1 || rc=$?
  if [[ "$rc" == "$want" ]]; then ok "$name"; else fail "$name: exit $rc"; sed 's/^/     /' "$T/out"; fi
}
owned_dirs() { find "$T" -maxdepth 1 -name '.archive.*'; }
children() { find "$1" -mindepth 1; }
archives_reported() { grep -c 'bundle sha256' "$T/out" || true; }
restores() { git clone -q --bare "$1" "$2" 2>/dev/null; }

printf 'someone else'\''s file\n' >"$T/by-id.bundle.partial"
expect "a bare object id is refused" 2 "$T/r" "$commit" "$T/by-id.bundle"
check "a refused archive leaves no bundle" test ! -e "$T/by-id.bundle"
check "an unrelated preexisting partial file is untouched by a refusal" grep -q "someone else's file" "$T/by-id.bundle.partial"
check "no owned directory is left behind" test -z "$(owned_dirs)"
ln -s "$T/elsewhere" "$T/link.bundle"
expect "a linked bundle path is refused" 2 "$T/r" refs/heads/main "$T/link.bundle"
check "the link is untouched" test -L "$T/link.bundle"
check "the link's target is untouched" test ! -e "$T/elsewhere"
expect "a short name is refused" 2 "$T/r" main "$T/short.bundle"
expect "a named ref is archived and restored" 0 "$T/r" refs/heads/main "$T/main.bundle"
check "the archive reports the exact commit, tree and private mode" grep -q "at $commit, tree $tree, bundle sha256 [0-9a-f]\{64\}, mode 600" "$T/out"
check "the bundle is private" test "$(stat -c %a "$T/main.bundle")" == 600
check "an independent restore succeeds" restores "$T/main.bundle" "$T/again"
check "the independent restore resolves the same commit" test "$(git -C "$T/again" rev-parse refs/heads/main)" == "$commit"
expect "an existing bundle path is never overwritten" 2 "$T/r" refs/heads/main "$T/main.bundle"
expect "a missing ref is refused" 2 "$T/r" refs/heads/absent "$T/absent.bundle"
# a damaged bundle: `git bundle verify` reads only the header and passes it, which is why the archive path's
# proof is the fresh restore, and that restore must fail
head -c 200 "$T/main.bundle" >"$T/damaged.bundle"
if restores "$T/damaged.bundle" "$T/from-damaged"; then fail "a damaged bundle restored"; else ok "a damaged bundle cannot be restored, so it is no archive"; fi
# a late writer: a git wrapper delegates every real Git operation unchanged, and after the fresh clone has finished
# it creates a directory, then a link to a directory, at the destination. The archive must refuse, write nothing
# into or through what arrived, report no archive, and leave no owned directory; the clean run after it passes.
mkdir -p "$T/wrap" "$T/foreign"; REAL_GIT="$(command -v git)"; FOREIGN="$T/foreign"; export REAL_GIT FOREIGN
cat >"$T/wrap/git" <<'WRAP'
#!/usr/bin/env bash
"$REAL_GIT" "$@"; rc=$?
if [[ "${1:-}" == clone && -n "${LATE_DEST:-}" ]]; then mkdir "$LATE_DEST"; fi
if [[ "${1:-}" == clone && -n "${LATE_LINK:-}" ]]; then ln -s "$FOREIGN" "$LATE_LINK"; fi
exit $rc
WRAP
chmod +x "$T/wrap/git"
rc=0; LATE_DEST="$T/late.bundle" PATH="$T/wrap:$PATH" "$S" "$T/r" refs/heads/main "$T/late.bundle" >"$T/out" 2>&1 || rc=$?
check "a directory arriving at the destination is refused" test "$rc" == 2
check "the arrived directory stays empty" test -d "$T/late.bundle" -a -z "$(children "$T/late.bundle")"
check "no archive is reported for the arrived directory" test "$(archives_reported)" == 0
rc=0; LATE_LINK="$T/late-link.bundle" PATH="$T/wrap:$PATH" "$S" "$T/r" refs/heads/main "$T/late-link.bundle" >"$T/out" 2>&1 || rc=$?
check "a link to a directory arriving at the destination is refused" test "$rc" == 2
check "the arrived link is untouched" test -L "$T/late-link.bundle"
check "nothing is written through the arrived link" test -z "$(children "$T/foreign")"
check "no archive is reported for the arrived link" test "$(archives_reported)" == 0
check "no owned directory is left after the late-writer refusals" test -z "$(owned_dirs)"
expect "the restored control archives after the late-writer cases" 0 "$T/r" refs/heads/main "$T/after.bundle"
check "the reported digest is the published bundle's" grep -q "bundle sha256 $(sha256sum "$T/after.bundle" | cut -d' ' -f1), mode 600" "$T/out"
check "the source repository is unchanged" test "$(fingerprint)" == "$before"
echo "archive controls: $failures failures"; exit "$failures"
