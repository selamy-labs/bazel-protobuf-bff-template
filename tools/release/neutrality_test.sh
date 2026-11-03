#!/usr/bin/env bash
# Witness for neutrality.sh (spec 010 FR-005 to FR-007): a clean scratch repository, then planted defects in
# contents, names, history, metadata and refs, and the private mode's statuses and file boundaries.
#
#   neutrality_test.sh <neutrality.sh>
set -euo pipefail
case "$1" in /*) S="$1" ;; *) S="$PWD/$1" ;; esac; T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
export GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL=a@example.com GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@example.com
printf 'example.com\n' >"$T/allow"
mk() { git init -q -b main "$1"; (cd "$1" && printf 'hello\n' >a.txt && git add . && git commit -q -m one); }
failures=0
expect() { local name="$1" want="$2"; shift 2; local rc=0; "$@" >"$T/out" 2>&1 || rc=$?; if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; sed 's/^/     /' "$T/out"; failures=$((failures + 1)); fi; }
mk "$T/clean"
expect "clean passes, private not-run" 0 "$S" "$T/clean" --allow "$T/allow"
grep -q 'private not-run' "$T/out" || { echo "FAIL not-run status missing"; failures=$((failures + 1)); }
: >"$T/empty"; expect "an empty marker file is not-run" 0 "$S" "$T/clean" --allow "$T/allow" --markers "$T/empty" --mapping "$T/map"
grep -q 'private not-run' "$T/out" || { echo "FAIL empty file read as a run"; failures=$((failures + 1)); }
# generic defects
mk "$T/g1"; (cd "$T/g1" && printf 'see /ho%s/x\n' 'me/someone' >b.txt && git add . && git commit -q -m two); expect "personal path in a current file" 1 "$S" "$T/g1" --allow "$T/allow"
mk "$T/g2"; (cd "$T/g2" && printf 'sa: thing@%s%s\n' 'proj.iam.gservice' 'account.com' >c.txt && git add . && git commit -q -m two && git rm -q c.txt && git commit -q -m three); expect "cloud identifier in a deleted file" 1 "$S" "$T/g2" --allow "$T/allow"
mk "$T/g3"; (cd "$T/g3" && git commit -q --allow-empty -m "contact: who@$(printf '%s.%s' private-corp example)"); expect "email domain in a message" 1 "$S" "$T/g3" --allow "$T/allow"
mk "$T/g4"; (cd "$T/g4" && git -c user.email=x@example.com commit -q --allow-empty -m ok); expect "allowed domain in an identity passes" 0 "$S" "$T/g4" --allow "$T/allow"
# private defects: a marker in a blob, a file name, a directory, a message, a ref, an identity
marker="zebra-orchid-7731"; printf '%s\n' "$marker" >"$T/markers"; chmod 600 "$T/markers"
mk "$T/p0"; expect "marker absent: private passed" 0 "$S" "$T/p0" --allow "$T/allow" --markers "$T/markers" --mapping "$T/map0"
grep -q 'private passed, 1 markers literally absent' "$T/out" || { echo "FAIL absent status"; failures=$((failures + 1)); }
plant() { # plant <name> <command run inside a fresh repo>
  mk "$T/$1"; (cd "$T/$1" && eval "$2"); expect "marker in $1" 1 "$S" "$T/$1" --allow "$T/allow" --markers "$T/markers" --mapping "$T/map-$1"
  if grep -qF "$marker" "$T/out"; then echo "FAIL the marker's text appeared in output ($1)"; failures=$((failures + 1)); fi
  grep -q 'private M1:' "$T/out" || { echo "FAIL no label for $1"; failures=$((failures + 1)); }
  [[ "$(stat -c %a "$T/map-$1")" == 600 ]] || { echo "FAIL mapping mode for $1"; failures=$((failures + 1)); }
}
plant blob "printf 'x $marker y\n' >d.txt && git add . && git commit -q -m two"
plant deleted-blob "printf '$marker\n' >d.txt && git add . && git commit -q -m two && git rm -q d.txt && git commit -q -m three"
plant file-name "printf 'x\n' >'$marker.txt' && git add . && git commit -q -m two"
plant directory "mkdir '$marker' && printf 'x\n' >'$marker/f' && git add . && git commit -q -m two"
plant message "git commit -q --allow-empty -m 'about $marker'"
plant ref "git branch 'topic/$marker'"
plant tag-message "git tag -a v1 -m 'release $marker'"
plant identity "GIT_AUTHOR_NAME='$marker' git commit -q --allow-empty -m two"
plant case "printf 'ZEBRA-ORCHID-7731\n' >d.txt && git add . && git commit -q -m two"
# the findings from the independent probe
plant large-blob "{ printf 'x $marker y\n'; head -c 2097152 /dev/zero | tr '\\0' 'a'; } >big.txt && git add . && git commit -q -m two"
mk "$T/clean-large"; (cd "$T/clean-large" && head -c 2097152 /dev/zero | tr '\0' 'a' >big.txt && git add . && git commit -q -m two); expect "a large blob with no marker: private passed" 0 "$S" "$T/clean-large" --allow "$T/allow" --markers "$T/markers" --mapping "$T/map-large-clean"
domain="fixture-secret.example"; printf '%s\n' "$domain" >"$T/domain-markers"; chmod 600 "$T/domain-markers"
mk "$T/leak"; (cd "$T/leak" && printf 'mail someone@%s\n' "$domain" >e.txt && git add . && git commit -q -m two)
expect "an email-domain marker" 1 "$S" "$T/leak" --allow "$T/allow" --markers "$T/domain-markers" --mapping "$T/map-leak"
if grep -qF "$domain" "$T/out"; then echo "FAIL the marker appeared in combined stdout and stderr"; failures=$((failures + 1)); fi
grep -q 'email domains outside the allowlist' "$T/out" || { echo "FAIL the generic category is missing"; failures=$((failures + 1)); }
printf '  \n\t\n' >"$T/blank"; expect "a whitespace-only marker file is not-run" 0 "$S" "$T/clean" --allow "$T/allow" --markers "$T/blank" --mapping "$T/map-blank"
grep -q 'private not-run (the marker file holds no terms)' "$T/out" || { echo "FAIL whitespace file was not not-run"; failures=$((failures + 1)); }
expect "a missing marker file is an error" 2 "$S" "$T/clean" --allow "$T/allow" --markers "$T/absent" --mapping "$T/map-absent"
touch "$T/existing-map"; chmod 644 "$T/existing-map"; expect "an existing mapping file is refused" 2 "$S" "$T/p0" --allow "$T/allow" --markers "$T/markers" --mapping "$T/existing-map"
expect "the marker file as the mapping path is refused" 2 "$S" "$T/p0" --allow "$T/allow" --markers "$T/markers" --mapping "$T/markers"
[[ "$(stat -c %s "$T/markers")" -gt 0 ]] || { echo "FAIL the marker file was truncated"; failures=$((failures + 1)); }
# the same content under two names: rev-list --objects shows one name, a raw tree listing shows both
plant two-names "printf 'same\n' >plain.txt && cp plain.txt '$marker.txt' && git add . && git commit -q -m two"
plant space-and-quote-name "printf 'x\n' >'a \"b\" $marker.txt' && git add . && git commit -q -m two"
ln -s "$T/elsewhere" "$T/link-map"; expect "a linked mapping path is refused" 2 "$S" "$T/p0" --allow "$T/allow" --markers "$T/markers" --mapping "$T/link-map"
expect "a mapping path inside the repository is refused" 2 "$S" "$T/p0" --allow "$T/allow" --markers "$T/markers" --mapping "$T/p0/map"
# objects reachable from a ref that is not a commit: a blob-only tag, a tag to a tree, an annotated chain
mk "$T/blob-tag"; (cd "$T/blob-tag" && b="$(printf 'x %s y\n' "$marker" | git hash-object -w --stdin)" && git tag blob-only "$b")
expect "marker in a blob named only by a lightweight tag" 1 "$S" "$T/blob-tag" --allow "$T/allow" --markers "$T/markers" --mapping "$T/map-blob-tag"
grep -q 'private M1:' "$T/out" || { echo "FAIL no label for the blob-only tag"; failures=$((failures + 1)); }
mk "$T/tree-tag"; (cd "$T/tree-tag" && mkdir t && printf 'x\n' >"t/$marker.txt" && git add t && tr="$(git write-tree)" && git rm -rqf t && git tag tree-only "$tr")
expect "marker in a path of a tree named only by a tag" 1 "$S" "$T/tree-tag" --allow "$T/allow" --markers "$T/markers" --mapping "$T/map-tree-tag"
mk "$T/chain"; (cd "$T/chain" && b="$(printf '%s\n' "$marker" | git hash-object -w --stdin)" && git tag -a inner -m inner "$b" && git tag -a outer -m outer inner)
expect "marker in a blob under an annotated tag chain" 1 "$S" "$T/chain" --allow "$T/allow" --markers "$T/markers" --mapping "$T/map-chain"
mk "$T/clean-tags"; (cd "$T/clean-tags" && b="$(printf 'plain\n' | git hash-object -w --stdin)" && git tag blob-only "$b" && git tag -a note -m 'a note' HEAD)
expect "tags without a marker still pass" 0 "$S" "$T/clean-tags" --allow "$T/allow" --markers "$T/markers" --mapping "$T/map-clean-tags"
# a decided object: printed with its reason, not a failure; the same text in a new blob is a new finding
mk "$T/decided"; (cd "$T/decided" && printf 'see /ho%s/x\n' 'me/someone' >b.txt && git add . && git commit -q -m two)
id="$(git -C "$T/decided" rev-parse HEAD:b.txt)"; { cat "$T/allow"; printf 'blob %s a synthetic fixture value\n' "$id"; } >"$T/allow-decided"
expect "a decided finding does not fail" 0 "$S" "$T/decided" --allow "$T/allow-decided"
grep -q 'decided at allowlist line [0-9]* of [0-9a-f]*' "$T/out" || { echo "FAIL the decision is not referenced"; failures=$((failures + 1)); }
grep -q 'a synthetic fixture value' "$T/out" && { echo "FAIL the allowlist's reason text was printed"; failures=$((failures + 1)); }
# a reason that holds a marker: never printed, in either scan's output
reason_marker="quokka-lantern-4420"; { cat "$T/allow"; printf 'blob %s decided because of %s\n' "$id" "$reason_marker"; } >"$T/allow-reason"; printf '%s\n' "$reason_marker" >"$T/reason-markers"; chmod 600 "$T/reason-markers"
expect "a decided reason that holds a marker" 0 "$S" "$T/decided" --allow "$T/allow-reason" --markers "$T/reason-markers" --mapping "$T/map-reason"
grep -qF "$reason_marker" "$T/out" && { echo "FAIL the reason's marker appeared in output"; failures=$((failures + 1)); }
grep -q 'private passed, 1 markers literally absent' "$T/out" || { echo "FAIL the export without the marker did not pass"; failures=$((failures + 1)); }
expect "the clean counterpart with the same allowlist" 0 "$S" "$T/decided" --allow "$T/allow-reason"
(cd "$T/decided" && printf 'see /ho%s/y\n' 'me/someone' >c.txt && git add . && git commit -q -m three)
expect "the same pattern in a new blob is a new finding" 1 "$S" "$T/decided" --allow "$T/allow-decided"

# Dynamic address helper to ensure test source contains no unallowed email literals
at="@"
gh_noreply="noreply${at}github.com"
foreign_bad="unallowed${at}foreign.example"
other_gh="other${at}github.com"

# 1) A decided commit object: printed with allowlist line and hash, not a failure; a new commit with unallowed domain still fails
mk "$T/commit-decided"; (cd "$T/commit-decided" && GIT_AUTHOR_EMAIL="$foreign_bad" git commit -q --allow-empty -m "historical commit")
c_id="$(git -C "$T/commit-decided" rev-parse HEAD)"
{ cat "$T/allow"; printf 'commit %s a decided historical commit fixture\n' "$c_id"; } >"$T/allow-commit-decided"
expect "a decided commit does not fail" 0 "$S" "$T/commit-decided" --allow "$T/allow-commit-decided"
grep -q 'decided at allowlist line [0-9]* of [0-9a-f]*' "$T/out" || { echo "FAIL the commit decision is not referenced"; failures=$((failures + 1)); }
grep -q 'a decided historical commit fixture' "$T/out" && { echo "FAIL the allowlist's reason text was printed"; failures=$((failures + 1)); }
(cd "$T/commit-decided" && GIT_AUTHOR_EMAIL="$foreign_bad" git commit -q --allow-empty -m "another commit")
expect "the same unallowed domain in a new commit is a new finding" 1 "$S" "$T/commit-decided" --allow "$T/allow-commit-decided"

# A decided commit allows email domain, but a personal-path finding on that commit still fails
mk "$T/commit-decided-with-path"
(cd "$T/commit-decided-with-path" && GIT_AUTHOR_EMAIL="$foreign_bad" git commit -q --allow-empty -m "$(printf 'path /Use%s/work in decided commit' 'rs/alice')")
c_path_id="$(git -C "$T/commit-decided-with-path" rev-parse HEAD)"
(cd "$T/commit-decided-with-path" && git commit -q --allow-empty -m "clean tip")
{ cat "$T/allow"; printf 'commit %s decided email on commit with path\n' "$c_path_id"; } >"$T/allow-commit-path"
expect "a decided commit with personal path still fails" 1 "$S" "$T/commit-decided-with-path" --allow "$T/allow-commit-path"

# Committer allowlist without at-sign: accepts exact committer on merge commits
{ cat "$T/allow"; printf 'committer noreply github.com GitHub merge committer\n'; } >"$T/allow-committer"

# 2) Committer allowlist: passes when committer email is noreply at github.com and author is allowed
mk "$T/committer-ok"
(cd "$T/committer-ok" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "merge commit")
expect "allowed author with noreply committer passes" 0 "$S" "$T/committer-ok" --allow "$T/allow-committer"

# Committer negative controls: unlisted local-part on allowed domain and unallowed foreign domain fail
mk "$T/committer-other-github"
(cd "$T/committer-other-github" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$other_gh" git commit -q --allow-empty -m "other committer")
expect "committer with unlisted local-part on allowed domain fails" 1 "$S" "$T/committer-other-github" --allow "$T/allow-committer"

mk "$T/committer-foreign"
(cd "$T/committer-foreign" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$foreign_bad" git commit -q --allow-empty -m "foreign committer")
expect "committer with unlisted domain fails" 1 "$S" "$T/committer-foreign" --allow "$T/allow-committer"

# 3) Disallowed author (noreply at github.com) fails even with committer allowlisted
mk "$T/committer-author-bad"
(cd "$T/committer-author-bad" && GIT_AUTHOR_EMAIL="$gh_noreply" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "bad author")
expect "noreply as author fails" 1 "$S" "$T/committer-author-bad" --allow "$T/allow-committer"

# Disallowed address in message fails
mk "$T/committer-msg-bad"
(cd "$T/committer-msg-bad" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "signed off by $gh_noreply")
expect "noreply in commit message fails" 1 "$S" "$T/committer-msg-bad" --allow "$T/allow-committer"

# Disallowed address in blob fails
mk "$T/committer-blob-bad"
(cd "$T/committer-blob-bad" && printf 'contact: %s\n' "$gh_noreply" >blob.txt && git add . && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q -m "blob with email")
expect "noreply in blob fails" 1 "$S" "$T/committer-blob-bad" --allow "$T/allow-committer"

# Private scan reads commit_full: a marker appearing only in an allowlisted committer field fails private scan
priv_committer_marker="$gh_noreply"
priv_gh_committer="NoReply${at}GitHub.com"
printf '%s\n' "$priv_committer_marker" >"$T/priv-committer-markers"; chmod 600 "$T/priv-committer-markers"
mk "$T/priv-committer-hit"
(cd "$T/priv-committer-hit" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$priv_gh_committer" git commit -q --allow-empty -m "clean commit")
expect "marker only in allowlisted committer fails private scan via commit_full" 1 "$S" "$T/priv-committer-hit" --allow "$T/allow-committer" --markers "$T/priv-committer-markers" --mapping "$T/map-priv-committer"
grep -q 'private failed' "$T/out" || { echo "FAIL private scan did not fail on allowlisted committer marker"; failures=$((failures + 1)); }
if grep -qF "$priv_committer_marker" "$T/out"; then echo "FAIL the marker's text appeared in output"; failures=$((failures + 1)); fi
if grep -qF "$priv_gh_committer" "$T/out"; then echo "FAIL committer literal appeared in output"; failures=$((failures + 1)); fi
[[ "$(stat -c %a "$T/map-priv-committer")" == 600 ]] || { echo "FAIL private committer mapping mode"; failures=$((failures + 1)); }

# 4) PR-time multi-commit ratchet: fails when ahead_count > 1 on PR events
mk "$T/ratchet-multi"
base_multi="$(git -C "$T/ratchet-multi" rev-parse HEAD)"
(cd "$T/ratchet-multi" && git commit -q --allow-empty -m "pr commit 1" && git commit -q --allow-empty -m "pr commit 2")

expect "multi-commit PR branch fails merge ratchet via GITHUB_EVENT_NAME=pull_request" 1 env GITHUB_EVENT_NAME=pull_request "$S" "$T/ratchet-multi" --allow "$T/allow-committer" --base "$base_multi"
grep -q 'ahead of base; squash to a single' "$T/out" || { echo "FAIL merge ratchet failure message missing (GITHUB_EVENT_NAME=pull_request)"; failures=$((failures + 1)); }

expect "multi-commit PR branch fails merge ratchet via --event pull_request" 1 "$S" "$T/ratchet-multi" --allow "$T/allow-committer" --base "$base_multi" --event pull_request

expect "multi-commit PR branch fails merge ratchet via --event=pull_request" 1 "$S" "$T/ratchet-multi" --allow "$T/allow-committer" "--base=$base_multi" --event=pull_request

# 5) Single-commit range (ahead_count == 1) passes ratchet
mk "$T/ratchet-single"
base_single="$(git -C "$T/ratchet-single" rev-parse HEAD)"
(cd "$T/ratchet-single" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "squashed pr commit")
expect "single squashed commit PR branch passes merge ratchet via --base and --event pull_request" 0 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --base "$base_single" --event pull_request
expect "single squashed commit PR branch passes merge ratchet via --base and GITHUB_EVENT_NAME=pull_request" 0 env GITHUB_EVENT_NAME=pull_request "$S" "$T/ratchet-single" --allow "$T/allow-committer" --base "$base_single"

# 6) Cumulative release range: multi-commit passes when event is push, merge_group, or unset
expect "multi-commit branch passes cumulative verification under --event push" 0 "$S" "$T/ratchet-multi" --allow "$T/allow-committer" --base "$base_multi" --event push
expect "multi-commit branch passes cumulative verification under --event merge_group" 0 "$S" "$T/ratchet-multi" --allow "$T/allow-committer" --base "$base_multi" --event merge_group
expect "multi-commit branch passes cumulative verification when event is unset" 0 "$S" "$T/ratchet-multi" --allow "$T/allow-committer" --base "$base_multi"

# 7) Ref and flag validation fails with exit 2
expect "empty --base value fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --base ""
expect "empty --base= fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --base=
expect "empty --event value fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --event ""
expect "empty --event= fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --event=
expect "unresolvable --base fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --base "no-such-ref-exists"
expect "unresolvable --base= fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" "--base=no-such-ref-exists"

# 8) Commit message containing literal \x1e one commit behind tip fails
mk "$T/inband-rec-sep"
(cd "$T/inband-rec-sep" && git commit -q --allow-empty -m "$(printf 'subject\x1e \x1fcontact: bad%sforeign.example' "$at")" && git commit -q --allow-empty -m "clean tip")
expect "commit message with literal record separator behind tip fails" 1 "$S" "$T/inband-rec-sep" --allow "$T/allow-committer"

# 9) Author name containing literal \x1f fails when author email is disallowed
mk "$T/inband-field-sep"
(cd "$T/inband-field-sep" && GIT_AUTHOR_NAME="$(printf 'bad\x1fname\x1fuser')" GIT_AUTHOR_EMAIL="$gh_noreply" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "field separator in author name")
expect "author name containing field separator with bad author email fails" 1 "$S" "$T/inband-field-sep" --allow "$T/allow-committer"

# 10) Quality registry config passes base and sets needs_base on neutrality check
reg="quality/registry.textproto"
if [[ ! -f "$reg" && -n "${RUNFILES_DIR:-}" && -f "${RUNFILES_DIR}/_main/quality/registry.textproto" ]]; then
  reg="${RUNFILES_DIR}/_main/quality/registry.textproto"
elif [[ ! -f "$reg" && -n "${TEST_SRCDIR:-}" && -f "${TEST_SRCDIR}/_main/quality/registry.textproto" ]]; then
  reg="${TEST_SRCDIR}/_main/quality/registry.textproto"
elif [[ ! -f "$reg" && -n "${RUNFILES_DIR:-}" && -f "${RUNFILES_DIR}/bff_template_quality+/registry.textproto" ]]; then
  reg="${RUNFILES_DIR}/bff_template_quality+/registry.textproto"
elif [[ ! -f "$reg" && -n "${TEST_SRCDIR:-}" && -f "${TEST_SRCDIR}/bff_template_quality+/registry.textproto" ]]; then
  reg="${TEST_SRCDIR}/bff_template_quality+/registry.textproto"
elif [[ ! -f "$reg" && -n "${RUNFILES_DIR:-}" && -f "${RUNFILES_DIR}/bff_template_quality/registry.textproto" ]]; then
  reg="${RUNFILES_DIR}/bff_template_quality/registry.textproto"
elif [[ ! -f "$reg" && -n "${TEST_SRCDIR:-}" && -f "${TEST_SRCDIR}/bff_template_quality/registry.textproto" ]]; then
  reg="${TEST_SRCDIR}/bff_template_quality/registry.textproto"
elif [[ ! -f "$reg" && -f "${0%/*}/../../quality/registry.textproto" ]]; then
  reg="${0%/*}/../../quality/registry.textproto"
fi

if [[ ! -f "$reg" ]]; then
  echo "FAIL quality/registry.textproto missing"
  failures=$((failures + 1))
else
  awk '
    /^checks \{/ { in_check=1; block=""; has_neutrality=0 }
    in_check {
      block = block "\n" $0
      if ($0 ~ /id:[[:space:]]*"neutrality"/) has_neutrality=1
    }
    in_check && /^\}/ {
      if (has_neutrality) {
        found_neutrality=1
        if (block !~ /"--base",[[:space:]]*"\{base\}"/) missing_base=1
        if (block !~ /needs_base:[[:space:]]*true/) missing_needs_base=1
      }
      in_check=0
    }
    END {
      if (!found_neutrality) { print "FAIL neutrality check block not found in " FILENAME; exit 1 }
      if (missing_base) { print "FAIL neutrality check block missing --base {base} in " FILENAME; exit 1 }
      if (missing_needs_base) { print "FAIL neutrality check block missing needs_base: true in " FILENAME; exit 1 }
    }
  ' "$reg" || {
    echo "FAIL registry.textproto does not satisfy neutrality check requirements"
    failures=$((failures + 1))
  }
fi

echo "fixture failures: $failures"; exit "$failures"
