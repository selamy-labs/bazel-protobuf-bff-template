#!/usr/bin/env bash
# Witness for neutrality.sh (spec 010 FR-005 to FR-007): a clean scratch repository, then planted defects in
# contents, names, history, metadata and refs, and the private mode's statuses and file boundaries.
#
#   neutrality_test.sh <neutrality.sh>
set -euo pipefail
unset GITHUB_EVENT_NAME GITHUB_EVENT_PATH GITHUB_ACTOR GITHUB_RUN_ID
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
bot_committer="github-bot${at}selamy.dev"
bot_noreply="346154065+selamy-labs-bot${at}users.noreply.github.com"
human_noreply="12345+pselamy${at}users.noreply.github.com"
human_selamy="pselamy${at}selamy.dev"
foreign_bad="unallowed${at}foreign.example"
other_gh="other${at}github.com"
personal_gmail="pselamy${at}gmail.com"
claude_author="claude-cir${at}selamy.dev"

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

# 6a) Simulated GitHub rebase-merge commit on main passes neutrality on push
mk "$T/rebase-merge-push"
(cd "$T/rebase-merge-push" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "simulated rebase-merge commit")
expect "simulated GitHub rebase-merge commit on main passes on push" 0 "$S" "$T/rebase-merge-push" --allow "$T/allow-committer" --event push

# 6b) Simulated GitHub squash-by-user commit on main fails neutrality on push
owner_bad="owner${at}personal-domain.invalid"
mk "$T/squash-by-user-push"
(cd "$T/squash-by-user-push" && GIT_AUTHOR_EMAIL="$owner_bad" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "simulated squash-by-user commit")
expect "simulated GitHub squash-by-user commit on main fails on push" 1 "$S" "$T/squash-by-user-push" --allow "$T/allow-committer" --event push

# 6c) PR branch fails merge ratchet if governance/github/main.tf allows squash merge
mk "$T/governance-squash"
mkdir -p "$T/governance-squash/governance/github"
cat >"$T/governance-squash/governance/github/main.tf" <<'EOF'
resource "github_repository" "template" {
  allow_merge_commit     = false
  allow_squash_merge     = true
  allow_rebase_merge     = false
  delete_branch_on_merge = true
}
EOF
(cd "$T/governance-squash" && git add governance/github/main.tf && git commit -q -m "initial governance")
base_gov="$(git -C "$T/governance-squash" rev-parse HEAD)"
(cd "$T/governance-squash" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "feature commit")
expect "PR branch fails merge ratchet when governance allows squash merge" 1 "$S" "$T/governance-squash" --allow "$T/allow-committer" --base "$base_gov" --event pull_request
grep -q 'governance/github/main.tf must enforce rebase-only merge' "$T/out" || { echo "FAIL expected governance rebase-only merge error message"; failures=$((failures + 1)); }

# 6d) PR branch passes merge ratchet when governance enforces rebase-only merge
mk "$T/governance-rebase"
mkdir -p "$T/governance-rebase/governance/github"
cat >"$T/governance-rebase/governance/github/main.tf" <<'EOF'
resource "github_repository" "template" {
  allow_merge_commit     = false
  allow_squash_merge     = false
  allow_rebase_merge     = true
  delete_branch_on_merge = true
}
EOF
(cd "$T/governance-rebase" && git add governance/github/main.tf && git commit -q -m "initial governance")
base_rebase="$(git -C "$T/governance-rebase" rev-parse HEAD)"
(cd "$T/governance-rebase" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "feature commit")
expect "PR branch passes merge ratchet when governance enforces rebase-only merge" 0 "$S" "$T/governance-rebase" --allow "$T/allow-committer" --base "$base_rebase" --event pull_request

# 6e) Simulating a rebase-by-personal-user commit fails neutrality
{ cat "$T/allow-committer"; printf 'selamy.dev\ncommitter github-bot selamy.dev GitHub bot rebase-merge committer identity observed at b2cd2b1f under selamy-labs-bot merger\n'; } >"$T/allow-bot"
GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" mk "$T/rebase-by-personal"
base_rebase_personal="$(git -C "$T/rebase-by-personal" rev-parse HEAD)"
(cd "$T/rebase-by-personal" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="Patrick Selamy" GIT_COMMITTER_EMAIL="$personal_gmail" git commit -q --allow-empty -m "rebase by personal user")
expect "simulating a rebase-by-personal-user commit fails neutrality" 1 "$S" "$T/rebase-by-personal" --allow "$T/allow-bot" --base "$base_rebase_personal" --event pull_request
grep -q 'email domains outside the allowlist at commit' "$T/out" || { echo "FAIL expected personal committer domain failure"; failures=$((failures + 1)); }

# 6f) PR event payload validation: reject non-bot PR user login and allow approved bot identities
printf '{"pull_request": {"user": {"login": "pselamy"}}}\n' >"$T/pr-event-bad.json"
printf '{"pull_request": {"user": {"login": "selamy-labs-bot"}}}\n' >"$T/pr-event-bot.json"
printf '{"pull_request": {"user": {"login": "claude-cir"}}}\n' >"$T/pr-event-claude.json"
printf '{"pull_request": {"user": {"login": "dependabot[bot]"}}}\n' >"$T/pr-event-dependabot.json"
printf '{"pull_request": {"user": {"login": "evil-app[bot]"}}}\n' >"$T/pr-event-evil-bot.json"
printf '{"merge_group": {"actor": {"login": "pselamy"}}}\n' >"$T/mg-event-human.json"
printf '{"merge_group": {"actor": {"login": "selamy-labs-bot"}}}\n' >"$T/mg-event-bot.json"
printf '{bad\n' >"$T/pr-event-malformed.json"
printf '{"pull_request": {"user": {}}}\n' >"$T/pr-event-missing-user.json"

mk "$T/pr-user-bad"
base_pr_bad="$(git -C "$T/pr-user-bad" rev-parse HEAD)"
(cd "$T/pr-user-bad" && GIT_AUTHOR_EMAIL="a@example.com" GIT_COMMITTER_EMAIL="$gh_noreply" git commit -q --allow-empty -m "feature commit")
expect "PR event payload with personal user login fails neutrality via --event-path" 1 "$S" "$T/pr-user-bad" --allow "$T/allow-committer" --base "$base_pr_bad" --event pull_request --event-path "$T/pr-event-bad.json"
grep -q 'pull_request user '\''pselamy'\'' is not permitted' "$T/out" || { echo "FAIL expected PR user login rejection message"; failures=$((failures + 1)); }

expect "PR event payload with personal user login fails neutrality via GITHUB_EVENT_PATH" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH="$T/pr-event-bad.json" "$S" "$T/pr-user-bad" --allow "$T/allow-committer" --base "$base_pr_bad"
grep -q 'pull_request user '\''pselamy'\'' is not permitted' "$T/out" || { echo "FAIL expected PR user login rejection message via env"; failures=$((failures + 1)); }

# 6g) Simulating rebase-by-bot commit with approved bot PR event payloads passes neutrality
GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" mk "$T/rebase-by-bot"
base_rebase_bot="$(git -C "$T/rebase-by-bot" rev-parse HEAD)"
(cd "$T/rebase-by-bot" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="$bot_committer" git commit -q --allow-empty -m "rebase by bot")
expect "rebase-by-bot commit with selamy-labs-bot payload passes neutrality via --event-path" 0 "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot" --event pull_request --event-path "$T/pr-event-bot.json"
expect "rebase-by-bot commit with selamy-labs-bot payload passes neutrality via env" 0 env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH="$T/pr-event-bot.json" GITHUB_ACTOR=selamy-labs-bot "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "rebase-by-bot commit with claude-cir payload passes neutrality via env" 0 env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH="$T/pr-event-claude.json" GITHUB_ACTOR=claude-cir "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "rebase-by-bot commit with dependabot[bot] payload passes neutrality via env" 0 env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH="$T/pr-event-dependabot.json" GITHUB_ACTOR="dependabot[bot]" "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"

# 6h) Negative controls for actor ratchet and payload fail-closed semantics
expect "GITHUB_ACTOR with personal user fails neutrality via env" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH="$T/pr-event-bot.json" GITHUB_ACTOR=pselamy "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "GITHUB_ACTOR=pselamy without GITHUB_EVENT_PATH fails neutrality" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_ACTOR=pselamy "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "unreadable GITHUB_EVENT_PATH fails neutrality" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH=/nonexistent/path GITHUB_ACTOR=selamy-labs-bot "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "malformed JSON in GITHUB_EVENT_PATH fails neutrality" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH="$T/pr-event-malformed.json" GITHUB_ACTOR=selamy-labs-bot "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "missing pull_request user login in payload fails neutrality" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH="$T/pr-event-missing-user.json" GITHUB_ACTOR=selamy-labs-bot "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "missing GITHUB_ACTOR in CI (GITHUB_RUN_ID set) fails neutrality" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_RUN_ID=12345 GITHUB_EVENT_PATH="$T/pr-event-bot.json" "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "missing GITHUB_EVENT_PATH in CI (GITHUB_RUN_ID set) fails neutrality" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_RUN_ID=12345 GITHUB_ACTOR=selamy-labs-bot "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "GITHUB_ACTOR with unapproved bot evil-app[bot] fails neutrality" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_ACTOR="evil-app[bot]" "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "PR event payload with evil-app[bot] fails neutrality" 1 env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH="$T/pr-event-evil-bot.json" GITHUB_ACTOR=selamy-labs-bot "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "merge_group with human actor pselamy fails neutrality" 1 env GITHUB_EVENT_NAME=merge_group GITHUB_ACTOR=pselamy "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "merge_group event payload with human actor pselamy fails neutrality" 1 env GITHUB_EVENT_NAME=merge_group GITHUB_EVENT_PATH="$T/mg-event-human.json" GITHUB_ACTOR=selamy-labs-bot "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"
expect "merge_group with approved bot actor passes neutrality" 0 env GITHUB_EVENT_NAME=merge_group GITHUB_EVENT_PATH="$T/mg-event-bot.json" GITHUB_ACTOR=selamy-labs-bot "$S" "$T/rebase-by-bot" --allow "$T/allow-bot" --base "$base_rebase_bot"

# 6i) Scratch-clone committer and author verification suite with isolated scratch repositories
repo_allow="tools/release/neutrality.allow"
[[ -f "$repo_allow" ]] || repo_allow="${0%/*}/neutrality.allow"
[[ -f "$repo_allow" ]] || repo_allow="${S%/*}/neutrality.allow"
if [[ ! -f "$repo_allow" && -n "${RUNFILES_DIR:-}" ]]; then
  repo_allow="${RUNFILES_DIR}/_main/tools/release/neutrality.allow"
fi
if [[ ! -f "$repo_allow" && -n "${TEST_SRCDIR:-}" ]]; then
  repo_allow="${TEST_SRCDIR}/_main/tools/release/neutrality.allow"
fi

[[ -f "$repo_allow" ]] || { echo "neutrality_test: cannot locate canonical neutrality.allow" >&2; exit 1; }
allow_canonical="$repo_allow"

scratch_template="$T/scratch-template"
if [[ -d .git || -f .git ]]; then
  git clone --shared -q . "$scratch_template"
else
  GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" mk "$scratch_template"
fi

run_isolated_scratch_case() {
  local case_id="$1"
  local author_email="$2"
  local committer_name="$3"
  local committer_email="$4"
  local want_rc="$5"
  local test_name="$6"
  local expected_diag="${7:-}"

  local case_dir="$T/scratch-$case_id"
  rm -rf "$case_dir"
  git clone --shared -q "$scratch_template" "$case_dir"
  local base_c
  base_c="$(git -C "$case_dir" rev-parse HEAD)"
  git -C "$case_dir" checkout -q -b "$case_id" "$base_c"
  GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$author_email" \
  GIT_COMMITTER_NAME="$committer_name" GIT_COMMITTER_EMAIL="$committer_email" \
    git -C "$case_dir" commit -q --allow-empty -m "case $case_id"
  local case_sha
  case_sha="$(git -C "$case_dir" rev-parse HEAD)"

  local out="$T/out-$case_id"
  local rc=0
  "$S" "$case_dir" --allow "$allow_canonical" --base "$base_c" >"$out" 2>&1 || rc=$?
  if [[ "$rc" -ne "$want_rc" ]]; then
    echo "FAIL $test_name: exit $rc (expected $want_rc)"
    cat "$out"
    failures=$((failures + 1))
  elif [[ "$want_rc" -eq 0 ]] && grep -qE "undecided hits|failed with [0-9]+ hit" "$out"; then
    echo "FAIL $test_name: unexpected failure hits in clean pass output"
    cat "$out"
    failures=$((failures + 1))
  elif [[ -n "$expected_diag" ]]; then
    if ! grep -F "$expected_diag" "$out" | grep -v "decided at" | grep -Fq "at commit $case_sha"; then
      echo "FAIL $test_name: diagnostic missing '$expected_diag at commit $case_sha' (or matched decided finding)"
      cat "$out"
      failures=$((failures + 1))
    else
      echo "ok   $test_name"
    fi
  else
    echo "ok   $test_name"
  fi
  rm -rf "$case_dir"
}

# 1) Author claude-cir, Committer observed selamy-labs-bot -> PASS (exit 0)
run_isolated_scratch_case "case1" "$claude_author" "selamy-labs-bot" "$bot_committer" 0 \
  "scratch-clone case 1: claude-cir author and observed selamy-labs-bot committer passes"

# 1b) Author claude-cir, Committer unapproved selamy-labs-bot noreply -> FAIL (exit != 0)
run_isolated_scratch_case "case1b" "$claude_author" "selamy-labs-bot" "$bot_noreply" 1 \
  "scratch-clone case 1b: unapproved noreply bot committer fails" \
  "email domains outside the allowlist"

# 2) Author claude-cir, Committer GitHub noreply -> PASS (exit 0)
run_isolated_scratch_case "case2" "$claude_author" "GitHub" "$gh_noreply" 0 \
  "scratch-clone case 2: claude-cir author and GitHub noreply committer passes"

# 3) Committer Patrick Selamy with human noreply email -> FAIL (exit != 0)
run_isolated_scratch_case "case3" "$claude_author" "Patrick Selamy" "$human_noreply" 1 \
  "scratch-clone case 3: human user noreply committer fails" \
  "email domains outside the allowlist"

# 4) Committer Patrick Selamy with human selamy.dev email -> FAIL (exit != 0)
run_isolated_scratch_case "case4" "$claude_author" "Patrick Selamy" "$human_selamy" 1 \
  "scratch-clone case 4: human user selamy.dev committer fails" \
  "unapproved selamy.dev committer identity in commit"

# 4b) Committer with other unapproved local part on selamy.dev -> FAIL (exit != 0)
run_isolated_scratch_case "case4b" "$claude_author" "Other Bot" "other-bot${at}selamy.dev" 1 \
  "scratch-clone case 4b: unapproved selamy.dev committer local-part fails" \
  "unapproved selamy.dev committer identity in commit"

# 5) Committer Patrick Selamy with human gmail email -> FAIL (exit != 0)
run_isolated_scratch_case "case5" "$claude_author" "Patrick Selamy" "$personal_gmail" 1 \
  "scratch-clone case 5: human user gmail committer fails" \
  "email domains outside the allowlist"

# 6) Bot author selamy-labs-bot noreply -> FAIL (exit != 0)
run_isolated_scratch_case "case6" "$bot_noreply" "selamy-labs-bot" "$bot_noreply" 1 \
  "scratch-clone case 6: bot author fails" \
  "email domains outside the allowlist"

# 7) Human user selamy.dev author -> FAIL (exit != 0)
run_isolated_scratch_case "case7" "$human_selamy" "claude-cir" "$claude_author" 1 \
  "scratch-clone case 7: human user selamy.dev author fails" \
  "unapproved selamy.dev author identity in commit"

# 7b) github-bot as author -> FAIL (exit != 0)
run_isolated_scratch_case "case7b" "$bot_committer" "claude-cir" "$claude_author" 1 \
  "scratch-clone case 7b: github-bot as author fails" \
  "unapproved selamy.dev author identity in commit"

# 8) Author claude-cir, Committer claude-cir -> PASS (exit 0)
run_isolated_scratch_case "case8" "$claude_author" "claude-cir" "$claude_author" 0 \
  "scratch-clone case 8: claude-cir author and claude-cir committer passes"

# 9) Author on anthropic.com -> FAIL (exit != 0)
run_isolated_scratch_case "case9" "someone${at}anthropic.com" "claude-cir" "$claude_author" 1 \
  "scratch-clone case 9: anthropic.com author fails" \
  "unapproved anthropic.com author identity in commit"

# 10) Committer on anthropic.com -> FAIL (exit != 0)
run_isolated_scratch_case "case10" "$claude_author" "someone" "someone${at}anthropic.com" 1 \
  "scratch-clone case 10: anthropic.com committer fails" \
  "unapproved anthropic.com committer identity in commit"

# 11) Author on reserved example.com -> FAIL (exit != 0)
run_isolated_scratch_case "case11" "evil${at}example.com" "claude-cir" "$claude_author" 1 \
  "scratch-clone case 11: reserved domain example.com author fails" \
  "unapproved reserved domain author identity in commit"

# 12) Committer on reserved example.invalid -> FAIL (exit != 0)
run_isolated_scratch_case "case12" "$claude_author" "evil" "evil${at}example.invalid" 1 \
  "scratch-clone case 12: reserved domain example.invalid committer fails" \
  "unapproved reserved domain committer identity in commit"

# 6j) Author/committer identity policy drift test
main_tf="governance/github/main.tf"
[[ -f "$main_tf" ]] || main_tf="${0%/*}/../../governance/github/main.tf"
[[ -f "$main_tf" ]] || main_tf="${S%/*}/../../governance/github/main.tf"
if [[ ! -f "$main_tf" && -n "${RUNFILES_DIR:-}" ]]; then
  main_tf="${RUNFILES_DIR}/_main/governance/github/main.tf"
fi
if [[ ! -f "$main_tf" && -n "${TEST_SRCDIR:-}" ]]; then
  main_tf="${TEST_SRCDIR}/_main/governance/github/main.tf"
fi

[[ -f "$main_tf" ]] || { echo "FAIL drift: cannot locate governance/github/main.tf" >&2; exit 1; }
[[ -f "$allow_canonical" ]] || { echo "FAIL drift: cannot locate canonical allowlist" >&2; exit 1; }

c_pattern_hcl="$(sed -nE '/committer_email_pattern[[:space:]]*\{/,/\}/s/.*pattern[[:space:]]*=[[:space:]]*"([^"]+)".*/\1/p' "$main_tf" | head -1)"
a_pattern_hcl="$(sed -nE '/commit_author_email_pattern[[:space:]]*\{/,/\}/s/.*pattern[[:space:]]*=[[:space:]]*"([^"]+)".*/\1/p' "$main_tf" | head -1)"
c_regex="${c_pattern_hcl//\\\\/\\}"
a_regex="${a_pattern_hcl//\\\\/\\}"

candidates=(
  "${claude_author} 1 1"
  "${bot_committer} 0 1"
  "${gh_noreply} 0 1"
  "${bot_noreply} 0 0"
  "${human_selamy} 0 0"
  "${human_noreply} 0 0"
  "${personal_gmail} 0 0"
  "noreply${at}anthropic.com 0 0"
  "evil${at}example.com 0 0"
  "evil${at}example.invalid 0 0"
)

drift_failed=0
cand_idx=0

for row in "${candidates[@]}"; do
  cand_idx=$((cand_idx + 1))
  read -r cand exp_a exp_c <<<"$row"

  ruleset_c=0
  if grep -qE "$c_regex" <<<"$cand"; then ruleset_c=1; fi
  if [[ "$ruleset_c" -ne "$exp_c" ]]; then
    echo "FAIL drift: ruleset committer pattern for $cand is $ruleset_c (expected $exp_c)"
    drift_failed=1
  fi

  ruleset_a=0
  if grep -qE "$a_regex" <<<"$cand"; then ruleset_a=1; fi
  if [[ "$ruleset_a" -ne "$exp_a" ]]; then
    echo "FAIL drift: ruleset author pattern for $cand is $ruleset_a (expected $exp_a)"
    drift_failed=1
  fi

  allow_c=0
  if [[ "$cand" == "$claude_author" ]] || awk -v ce="$cand" '$1 == "committer" && tolower($2 "@" $3) == ce {found=1; exit} END {exit !found}' "$allow_canonical"; then
    allow_c=1
  fi
  if [[ "$allow_c" -ne "$exp_c" ]]; then
    echo "FAIL drift: allowlist committer acceptance for $cand is $allow_c (expected $exp_c)"
    drift_failed=1
  fi

  # Committer acceptance evaluated in isolated clone
  drift_case_c="$T/drift-c-$cand_idx"
  git clone --shared -q "$scratch_template" "$drift_case_c"
  drift_base_c="$(git -C "$drift_case_c" rev-parse HEAD)"
  git -C "$drift_case_c" checkout -q -b "drift-c" "$drift_base_c"
  GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" \
  GIT_COMMITTER_NAME="Committer" GIT_COMMITTER_EMAIL="$cand" \
    git -C "$drift_case_c" commit -q --allow-empty -m "drift test committer"
  neut_c=0
  if "$S" "$drift_case_c" --allow "$allow_canonical" --base "$drift_base_c" >/dev/null 2>&1; then
    neut_c=1
  fi
  rm -rf "$drift_case_c"
  if [[ "$neut_c" -ne "$exp_c" ]]; then
    echo "FAIL drift: neutrality.sh committer execution acceptance for $cand is $neut_c (expected $exp_c)"
    drift_failed=1
  fi

  # Author acceptance evaluated in isolated clone
  drift_case_a="$T/drift-a-$cand_idx"
  git clone --shared -q "$scratch_template" "$drift_case_a"
  drift_base_a="$(git -C "$drift_case_a" rev-parse HEAD)"
  git -C "$drift_case_a" checkout -q -b "drift-a" "$drift_base_a"
  GIT_AUTHOR_NAME="Author" GIT_AUTHOR_EMAIL="$cand" \
  GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" \
    git -C "$drift_case_a" commit -q --allow-empty -m "drift test author"
  neut_a=0
  if "$S" "$drift_case_a" --allow "$allow_canonical" --base "$drift_base_a" >/dev/null 2>&1; then
    neut_a=1
  fi
  rm -rf "$drift_case_a"
  if [[ "$neut_a" -ne "$exp_a" ]]; then
    echo "FAIL drift: neutrality.sh author execution acceptance for $cand is $neut_a (expected $exp_a)"
    drift_failed=1
  fi
done

if [[ "$drift_failed" -eq 0 ]]; then
  echo "ok   author and committer identity policy drift test passes across ruleset, neutrality.allow, and neutrality.sh"
else
  failures=$((failures + 1))
fi

rm -rf "$scratch_template"

# 7) Ref and flag validation fails with exit 2
expect "empty --base value fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --base ""
expect "empty --base= fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --base=
expect "empty --event value fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --event ""
expect "empty --event= fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --event=
expect "empty --event-path value fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --event-path ""
expect "empty --event-path= fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --event-path=
expect "unreadable --event-path fails with exit 2" 2 "$S" "$T/ratchet-single" --allow "$T/allow-committer" --event-path "$T/nonexistent-event-file"
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

# 11) Merge-path ratchet: verify_rebase_merge.sh and PR-time simulated rebase merge checks
vrm_sh="tools/release/verify_rebase_merge.sh"
[[ -f "$vrm_sh" ]] || vrm_sh="${0%/*}/verify_rebase_merge.sh"
[[ -f "$vrm_sh" ]] || vrm_sh="${S%/*}/verify_rebase_merge.sh"
if [[ ! -f "$vrm_sh" && -n "${RUNFILES_DIR:-}" ]]; then
  vrm_sh="${RUNFILES_DIR}/_main/tools/release/verify_rebase_merge.sh"
fi
if [[ ! -f "$vrm_sh" && -n "${TEST_SRCDIR:-}" ]]; then
  vrm_sh="${TEST_SRCDIR}/_main/tools/release/verify_rebase_merge.sh"
fi

if [[ ! -f "$vrm_sh" ]]; then
  echo "FAIL tools/release/verify_rebase_merge.sh missing"
  failures=$((failures + 1))
else
  # Create a clean fixture repo for verify_rebase_merge testing
  git init -q -b main "$T/vrm-repo"
  (cd "$T/vrm-repo" && printf 'hello\n' >a.txt && git add . && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" git commit -q -m one)
  mkdir -p "$T/vrm-repo/governance/github"
  cat >"$T/vrm-repo/governance/github/main.tf" <<'EOFG'
resource "github_repository" "template" {
  allow_merge_commit     = false
  allow_squash_merge     = false
  allow_rebase_merge     = true
  delete_branch_on_merge = true
}
EOFG
  (cd "$T/vrm-repo" && git add governance/github/main.tf && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" git commit -q -m "initial governance")
  vrm_base="$(git -C "$T/vrm-repo" rev-parse HEAD)"

  # A clean candidate commit by claude-cir
  (cd "$T/vrm-repo" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" git commit -q --allow-empty -m "candidate pr commit")
  vrm_head="$(git -C "$T/vrm-repo" rev-parse HEAD)"

  # Negative control (BFF-RM12-03, SOL-R13-03): fails closed if no bot rebase-merge commit exists in observed history
  expect "verify_rebase_merge fails closed when no bot merge observed in history" 1 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "$vrm_head" --base "$vrm_base"
  expect "verify_rebase_merge fails closed when no bot merge observed even with explicit matching --bot-committer" 1 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "$vrm_head" --base "$vrm_base" --bot-committer "github-bot@selamy.dev"
  expect "verify_rebase_merge fails closed when no bot merge observed with explicit noreply --bot-committer" 1 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "$vrm_head" --base "$vrm_base" --bot-committer "$gh_noreply"
  expect "PR neutrality scan fails closed when no bot merge observed in history" 1 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_base" --event pull_request

  # Now add observed bot merge commit to history, matching b2cd2b1f (selamy-labs-bot <github-bot@selamy.dev>)
  (cd "$T/vrm-repo" && git checkout -q -b bot-base "$vrm_base" && \
    GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" \
    GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="$bot_committer" \
    git commit -q --allow-empty -m "observed bot merge on base")
  vrm_bot_base="$(git -C "$T/vrm-repo" rev-parse bot-base)"

  (cd "$T/vrm-repo" && git checkout -q -b pr-branch "$vrm_bot_base" && \
    GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" \
    GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" \
    git commit -q --allow-empty -m "candidate pr commit on bot base")
  vrm_bot_head="$(git -C "$T/vrm-repo" rev-parse pr-branch)"

  # Canonical allowlist: verify_rebase_merge passes with observed bot merger (exit 0)
  expect "verify_rebase_merge passes on clean candidate with observed bot merger" 0 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "$vrm_bot_head" --base "$vrm_bot_base"
  expect "verify_rebase_merge passes when explicit --bot-committer matches observed bot committer" 0 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "$vrm_bot_head" --base "$vrm_bot_base" --bot-committer "$bot_committer"
  expect "verify_rebase_merge fails when explicit --bot-committer mismatches observed bot committer" 1 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "$vrm_bot_head" --base "$vrm_bot_base" --bot-committer "mismatch@selamy.dev"
  expect "verify_rebase_merge fails when explicit --bot-name mismatches observed bot name" 1 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "$vrm_bot_head" --base "$vrm_bot_base" --bot-name "mismatch-bot"

  # PR-time neutrality scan passes on clean candidate with observed bot merger
  expect "PR-time neutrality scan passes simulated rebase merge with observed bot merger" 0 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --event pull_request

  # Negative control: allowlist lacking committer github-bot selamy.dev
  grep -v "committer github-bot selamy.dev" "$allow_canonical" >"$T/allow-no-bot"
  expect "verify_rebase_merge fails when allowlist lacks github-bot committer" 1 "$vrm_sh" --repo "$T/vrm-repo" --allow "$T/allow-no-bot" --head "$vrm_bot_head" --base "$vrm_bot_base"

  # Negative control: PR-time neutrality scan fails when allowlist lacks github-bot committer
  expect "PR-time neutrality scan fails when allowlist lacks github-bot committer" 1 "$S" "$T/vrm-repo" --allow "$T/allow-no-bot" --base "$vrm_bot_base" --event pull_request

  # Negative control (ARGON-BFF-R12-002): decoy allowlist with selamy-labs-bot in wrong local-part or comment must not be picked up
  cat >"$T/allow-decoy-bot" <<'EOFDEC'
selamy.dev
author claude-cir selamy.dev
committer selamy-labs-bot selamy.dev decoy wrong local part mentioning selamy-labs-bot
committer decoy-bot selamy.dev comment referencing selamy-labs-bot
EOFDEC
  expect "verify_rebase_merge fails when allowlist only has decoy selamy-labs-bot entry" 1 "$vrm_sh" --repo "$T/vrm-repo" --allow "$T/allow-decoy-bot" --head "$vrm_bot_head" --base "$vrm_bot_base"
  expect "PR-time neutrality scan fails when allowlist only has decoy selamy-labs-bot entry" 1 "$S" "$T/vrm-repo" --allow "$T/allow-decoy-bot" --base "$vrm_bot_base" --event pull_request

  # Negative control: candidate commit with personal author identity fails verify_rebase_merge and neutrality PR simulation
  vrm_bad_repo="$T/vrm-bad-repo"
  git clone -q "$T/vrm-repo" "$vrm_bad_repo"
  (cd "$vrm_bad_repo" && git checkout -q -b bad-pr "$vrm_bot_base" && \
    GIT_AUTHOR_NAME="Patrick Selamy" GIT_AUTHOR_EMAIL="$human_selamy" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" git commit -q --allow-empty -m "bad author candidate")
  vrm_bad_head="$(git -C "$vrm_bad_repo" rev-parse bad-pr)"
  expect "verify_rebase_merge fails candidate commit with unapproved selamy.dev author" 1 "$vrm_sh" --repo "$vrm_bad_repo" --allow "$allow_canonical" --head "$vrm_bad_head" --base "$vrm_bot_base"
  expect "PR-time neutrality scan fails candidate commit with unapproved selamy.dev author" 1 "$S" "$vrm_bad_repo" --allow "$allow_canonical" --base "$vrm_bot_base" --event pull_request
  rm -rf "$vrm_bad_repo"

  # Regression test for ARGON-BFF-R13-002: newer selamy-labs-bot commit with unapproved email must not fall back to older approved commit
  vrm_newer_bad_bot_repo="$T/vrm-newer-bad-bot"
  git clone -q "$T/vrm-repo" "$vrm_newer_bad_bot_repo"
  (cd "$vrm_newer_bad_bot_repo" && \
    git checkout -q -b newer-bot-base "$vrm_bot_base" && \
    GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="unapproved-bot@selamy.dev" git commit -q --allow-empty -m "newer bot merge commit with unapproved email" && \
    git checkout -q -b newer-pr && \
    GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="$claude_author" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="$claude_author" git commit -q --allow-empty -m "clean candidate on newer bot base")
  vrm_newer_base="$(git -C "$vrm_newer_bad_bot_repo" rev-parse newer-bot-base)"
  vrm_newer_head="$(git -C "$vrm_newer_bad_bot_repo" rev-parse newer-pr)"
  expect "verify_rebase_merge fails closed when newer bot merge has unapproved email" 1 "$vrm_sh" --repo "$vrm_newer_bad_bot_repo" --allow "$allow_canonical" --head "$vrm_newer_head" --base "$vrm_newer_base"
  expect "PR neutrality scan fails closed when newer bot merge has unapproved email" 1 "$S" "$vrm_newer_bad_bot_repo" --allow "$allow_canonical" --base "$vrm_newer_base" --event pull_request
  rm -rf "$vrm_newer_bad_bot_repo"

  # Negative controls (BFF-RM12-SOL-03): verify_rebase_merge fails closed on simulation setup errors
  expect "verify_rebase_merge fails closed on unresolvable base" 2 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "$vrm_bot_head" --base "nonexistent-base-1234"
  expect "verify_rebase_merge fails closed on unresolvable head" 2 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "nonexistent-head-1234" --base "$vrm_bot_base"
  expect "verify_rebase_merge fails closed on unreadable allowlist" 2 "$vrm_sh" --repo "$T/vrm-repo" --allow "$T/nonexistent-allow" --head "$vrm_bot_head" --base "$vrm_bot_base"
  mkdir -p "$T/not-a-repo"
  expect "verify_rebase_merge fails closed on non-git directory" 2 "$vrm_sh" --repo "$T/not-a-repo" --allow "$allow_canonical" --head "$vrm_bot_head" --base "$vrm_bot_base"

  # Fault-injection regression tests for PR simulation (SOL-R13-04):
  # 1. Unresolvable base in PR mode fails with exit 2
  expect "PR neutrality scan fails on unresolvable base" 2 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "nonexistent-base-1234" --event pull_request

  # 2. Unreadable allowlist in PR mode fails with exit 2
  expect "PR neutrality scan fails on unreadable allowlist" 2 "$S" "$T/vrm-repo" --allow "$T/nonexistent-allow" --base "$vrm_bot_base" --event pull_request

  # 3. Failed scratch clone during PR simulation fails with exit 1 and diagnostic
  mkdir -p "$T/mock-bin-clone-fail"
  cat >"$T/mock-bin-clone-fail/git" <<'EOFCLONE'
#!/usr/bin/env bash
for arg in "$@"; do
  if [[ "$arg" == "clone" ]]; then
    echo "fatal: injected clone failure" >&2
    exit 128
  fi
done
exec /usr/bin/git "$@"
EOFCLONE
  chmod +x "$T/mock-bin-clone-fail/git"
  expect "PR neutrality scan fails when scratch clone fails" 1 env PATH="$T/mock-bin-clone-fail:$PATH" "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --event pull_request
  grep -qF "failed to clone scratch repository for rebase-merge simulation" "$T/out" || {
    echo "FAIL missing scratch clone failure diagnostic in output"; failures=$((failures + 1))
  }

  # 4. Failed candidate tree resolution during PR simulation fails with exit 1 and diagnostic
  mkdir -p "$T/mock-bin-tree-fail"
  cat >"$T/mock-bin-tree-fail/git" <<'EOFTREE'
#!/usr/bin/env bash
for arg in "$@"; do
  if [[ "$arg" == *"^{tree}"* ]]; then
    echo "fatal: injected tree resolution failure" >&2
    exit 128
  fi
done
exec /usr/bin/git "$@"
EOFTREE
  chmod +x "$T/mock-bin-tree-fail/git"
  expect "PR neutrality scan fails when tree resolution fails" 1 env PATH="$T/mock-bin-tree-fail:$PATH" "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --event pull_request
  grep -qF "failed to resolve candidate tree for rebase-merge simulation" "$T/out" || {
    echo "FAIL missing candidate tree failure diagnostic in output"; failures=$((failures + 1))
  }

  # 5. Failed synthetic commit creation during PR simulation fails with exit 1 and diagnostic
  mkdir -p "$T/mock-bin-commit-fail"
  cat >"$T/mock-bin-commit-fail/git" <<'EOFCOMMIT'
#!/usr/bin/env bash
for arg in "$@"; do
  if [[ "$arg" == "commit-tree" ]]; then
    echo "fatal: injected commit-tree failure" >&2
    exit 128
  fi
done
exec /usr/bin/git "$@"
EOFCOMMIT
  chmod +x "$T/mock-bin-commit-fail/git"
  expect "PR neutrality scan fails when commit creation fails" 1 env PATH="$T/mock-bin-commit-fail:$PATH" "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --event pull_request
  grep -qF "failed to create synthetic commit for rebase-merge simulation" "$T/out" || {
    echo "FAIL missing commit creation failure diagnostic in output"; failures=$((failures + 1))
  }

  # 6. Crashing inner neutrality scan during PR simulation fails with exit 1 and diagnostic
  cp "$S" "$T/injected-crash-neutrality.sh"
  sed -i '2i if [[ "$*" == *"--internal-simulation"* ]]; then echo "crash: injected inner push scan failure" >&2; exit 42; fi' "$T/injected-crash-neutrality.sh"
  chmod +x "$T/injected-crash-neutrality.sh"
  expect "PR neutrality scan fails when inner neutrality scan crashes" 1 "$T/injected-crash-neutrality.sh" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --event pull_request
  grep -qF "fails neutrality scan" "$T/out" || {
    echo "FAIL missing inner scan crash diagnostic in output"; failures=$((failures + 1))
  }

  # Regression test for BFF-RM12-SOL-01 / BFF-RM12-01: exported CI environment does not cause recursion
  pr_payload_file="$T/pr-event.json"
  printf '{"pull_request":{"user":{"login":"claude-cir"}}}\n' >"$pr_payload_file"
  pr_rec_out="$T/out-pr-recursion"
  pr_rec_rc=0
  (
    export GITHUB_ACTIONS="true"
    export GITHUB_EVENT_NAME="pull_request"
    export GITHUB_ACTOR="claude-cir"
    export GITHUB_EVENT_PATH="$pr_payload_file"
    timeout 15 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" >"$pr_rec_out" 2>&1
  ) || pr_rec_rc=$?
  if [[ "$pr_rec_rc" -eq 0 ]]; then
    echo "ok   PR neutrality scan in CI environment terminates without recursion"
  else
    echo "FAIL PR neutrality scan in CI environment failed: exit $pr_rec_rc"
    cat "$pr_rec_out"
    failures=$((failures + 1))
  fi

  # Single-commit simulation scope test (BFF-RM14-01, SOL-R14-02, ARGON-BFF-R14-001)
  # 1. verify_rebase_merge outputs scoped simulation output ('scope: 1 commit, N changed blobs')
  expect "verify_rebase_merge passes and outputs scoped simulation" 0 "$vrm_sh" --repo "$T/vrm-repo" --allow "$allow_canonical" --head "$vrm_bot_head" --base "$vrm_bot_base"
  grep -qE "scope: 1 commit, [0-9]+ changed blobs, [0-9]+ distinct paths, 0 refs, all identities" "$T/out" || {
    echo "FAIL verify_rebase_merge did not report single-commit scope in output"; failures=$((failures + 1))
  }

  # 2. neutrality.sh PR mode outputs scoped simulation info
  expect "PR neutrality scan reports scoped simulated rebase-merge" 0 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --event pull_request
  grep -qE "simulated rebase-merge \(scope: 1 commit, [0-9]+ changed blobs" "$T/out" || {
    echo "FAIL PR neutrality scan did not report scoped rebase-merge in output"; failures=$((failures + 1))
  }

  # 3. Behavioral proof: neutrality.sh --commit only scans target commit and changed blobs, not unrelated dirty history
  dirty_repo="$T/dirty-history-repo"
  git clone -q "$T/vrm-repo" "$dirty_repo"
  fake_key="AKIA$(printf '%s%s' 'IOSFODNN' '7EXAMPLE')"
  (cd "$dirty_repo" && git checkout -q -b dirty-branch && printf 'secret=%s\n' "$fake_key" >secret.txt && git add secret.txt && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q -m "dirty commit with secret")
  git -C "$dirty_repo" checkout -q main

  # Full push-mode scan fails on dirty-branch
  expect "full push scan fails on dirty history in other ref" 1 "$S" "$dirty_repo" --allow "$allow_canonical" --base "$vrm_bot_base" --event push
  grep -qF "secret pattern at blob" "$T/out" || {
    echo "FAIL expected secret pattern detection in full scan"; failures=$((failures + 1))
  }

  # Targeted --commit scan passes because it only scans the candidate commit and changed blobs
  expect "targeted commit scan ignores unrelated dirty history" 0 "$S" "$dirty_repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head" --internal-simulation
  grep -qE "scope: 1 commit, [0-9]+ changed blobs, [0-9]+ distinct paths, 0 refs, all identities" "$T/out" || {
    echo "FAIL expected scoped scan output for targeted commit scan"; failures=$((failures + 1))
  }
  rm -rf "$dirty_repo"

  # Negative test for regex metacharacters in allowlist committer matching (BFF-RM14-03)
  # selamyXdev decoy line: "." in regex would match "X", but exact-field match must reject
  decoy_allow="$T/decoy-metachar.allow"
  sed 's/selamy\.dev/selamyXdev/g' "$allow_canonical" >"$decoy_allow"

  # verify_rebase_merge.sh pre-check fails closed with exit 1
  expect "verify_rebase_merge fails closed on regex metacharacter decoy allowlist" 1 "$vrm_sh" --repo "$T/vrm-repo" --allow "$decoy_allow" --head "$vrm_bot_head" --base "$vrm_bot_base"
  grep -qF "does not permit observed bot merger committer identity" "$T/out" || {
    echo "FAIL missing pre-check rejection message in verify_rebase_merge with decoy allowlist"; failures=$((failures + 1))
  }

  # neutrality.sh PR mode pre-check fails closed with exit 1
  expect "PR neutrality scan fails closed on regex metacharacter decoy allowlist" 1 "$S" "$T/vrm-repo" --allow "$decoy_allow" --base "$vrm_bot_base" --event pull_request
  grep -qF "neutrality.allow must accept observed bot merger committer identity (github-bot@selamy.dev)" "$T/out" || {
    echo "FAIL missing pre-check rejection message in PR neutrality with decoy allowlist"; failures=$((failures + 1))
  }
  rm -f "$decoy_allow"

  # SOL-R15-01 / BFF-RM15-01 negative tests: --commit option must be restricted to internal simulation only
  # 1. --commit without --internal-simulation exits 2
  expect "--commit without --internal-simulation fails closed with exit 2" 2 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head"
  grep -qF "permitted only for internal simulation" "$T/out" || {
    echo "FAIL expected internal simulation required message in output"; failures=$((failures + 1))
  }

  # 2. --commit plus --event pull_request exits 2
  expect "--commit plus --event pull_request fails closed with exit 2" 2 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head" --event pull_request
  grep -qF "cannot be combined with --event pull_request" "$T/out" || {
    echo "FAIL expected cannot combine with --event pull_request message in output"; failures=$((failures + 1))
  }

  # 3. --commit --internal-simulation plus --event pull_request exits 2
  expect "--commit --internal-simulation plus --event pull_request fails closed with exit 2" 2 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head" --internal-simulation --event pull_request
  grep -qF "cannot be combined with --event pull_request" "$T/out" || {
    echo "FAIL expected cannot combine with --event pull_request message in output"; failures=$((failures + 1))
  }

  # 4. --commit plus --event push exits 2
  expect "--commit plus --event push fails closed with exit 2" 2 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head" --event push
  grep -qF "cannot be combined with --event push" "$T/out" || {
    echo "FAIL expected cannot combine with --event push message in output"; failures=$((failures + 1))
  }

  # 5. --commit --internal-simulation plus --event push exits 2
  expect "--commit --internal-simulation plus --event push fails closed with exit 2" 2 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head" --internal-simulation --event push
  grep -qF "cannot be combined with --event push" "$T/out" || {
    echo "FAIL expected cannot combine with --event push message in output"; failures=$((failures + 1))
  }

  # 6. --commit in GitHub Actions CI environment exits 2
  expect "--commit under GITHUB_ACTIONS=true fails closed with exit 2" 2 env GITHUB_ACTIONS="true" "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head" --internal-simulation
  grep -qF "cannot be used in a GitHub Actions CI environment" "$T/out" || {
    echo "FAIL expected CI environment rejection in output"; failures=$((failures + 1))
  }

  expect "--commit under GITHUB_RUN_ID fails closed with exit 2" 2 env GITHUB_RUN_ID="12345" "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head" --internal-simulation
  grep -qF "cannot be used in a GitHub Actions CI environment" "$T/out" || {
    echo "FAIL expected CI environment rejection in output for GITHUB_RUN_ID"; failures=$((failures + 1))
  }

  expect "--commit under GITHUB_EVENT_NAME=push fails closed with exit 2" 2 env GITHUB_EVENT_NAME="push" "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head" --internal-simulation
  grep -qE "cannot be (combined with --event|used in a GitHub Actions CI environment)" "$T/out" || {
    echo "FAIL expected event or CI rejection in output for GITHUB_EVENT_NAME"; failures=$((failures + 1))
  }

  # 7. Multi-commit PR with human actor is still rejected in PR mode (without --commit)
  multi_pr_repo="$T/multi-pr-repo"
  git clone -q "$T/vrm-repo" "$multi_pr_repo"
  (cd "$multi_pr_repo" && git checkout -q -b multi-pr-branch "$vrm_bot_base" && \
   printf 'commit1\n' >file1.txt && git add file1.txt && \
   GIT_AUTHOR_NAME="Alice Human" GIT_AUTHOR_EMAIL="alice@example.com" GIT_COMMITTER_NAME="Alice Human" GIT_COMMITTER_EMAIL="alice@example.com" git commit -q -m "first PR commit" && \
   printf 'commit2\n' >file2.txt && git add file2.txt && \
   GIT_AUTHOR_NAME="Alice Human" GIT_AUTHOR_EMAIL="alice@example.com" GIT_COMMITTER_NAME="Alice Human" GIT_COMMITTER_EMAIL="alice@example.com" git commit -q -m "second PR commit")
  expect "multi-commit PR with human actor is rejected in PR mode" 1 "$S" "$multi_pr_repo" --allow "$allow_canonical" --base "$vrm_bot_base" --event pull_request
  grep -qF "commits ahead of base; squash to a single neutral-author commit" "$T/out" || {
    echo "FAIL expected multi-commit ratchet failure in output"; failures=$((failures + 1))
  }
  rm -rf "$multi_pr_repo"

  # 8. Pin test: quality/registry.textproto neutrality command does not pass simulation flags
  repo_root="$(cd "$(dirname "$S")/../.." 2>/dev/null && pwd || true)"
  if [[ -f "$repo_root/quality/registry.textproto" ]]; then
    if grep -qF -- "--commit" "$repo_root/quality/registry.textproto"; then
      echo "FAIL quality/registry.textproto neutrality command contains --commit"; failures=$((failures + 1))
    fi
    if grep -qF -- "--internal-simulation" "$repo_root/quality/registry.textproto"; then
      echo "FAIL quality/registry.textproto neutrality command contains --internal-simulation"; failures=$((failures + 1))
    fi
    echo "ok   quality/registry.textproto neutrality command contains no simulation flags"
  fi

  # 9. Private marker scan in simulation mode (ARGON-BFF-R15-001)
  # paths.zu and refs are populated; clean run succeeds
  sim_marker_file="$T/sim-markers.txt"
  sim_map_file="$T/sim-mapping.txt"
  printf '%s\n' "secret-sim-marker-absent" >"$sim_marker_file"
  expect "marker scan succeeds in simulation mode when marker is absent" 0 "$S" "$T/vrm-repo" --allow "$allow_canonical" --base "$vrm_bot_base" --commit "$vrm_bot_head" --internal-simulation --markers "$sim_marker_file" --mapping "$sim_map_file"
  grep -qF "private passed" "$T/out" || {
    echo "FAIL expected private passed in simulation marker scan"; failures=$((failures + 1))
  }
  rm -f "$sim_marker_file" "$sim_map_file"

  # Pin b2cd2b1f's actual committer as observed in repository history (BFF-RM12-03)
  if git rev-parse --verify --quiet "b2cd2b1f3f9a2bdf1450d99b8c446d2b5095a40f^{commit}" >/dev/null 2>&1; then
    b2_cn="$(git log -1 --format='%cn' b2cd2b1f3f9a2bdf1450d99b8c446d2b5095a40f)"
    b2_ce="$(git log -1 --format='%ce' b2cd2b1f3f9a2bdf1450d99b8c446d2b5095a40f)"
    if [[ "$b2_cn" == "selamy-labs-bot" && "$b2_ce" == "github-bot@selamy.dev" ]]; then
      echo "ok   b2cd2b1f committer pinned to observed identity selamy-labs-bot <github-bot@selamy.dev>"
    else
      echo "FAIL b2cd2b1f committer mismatch: got '$b2_cn <$b2_ce>'"
      failures=$((failures + 1))
    fi
  fi
fi

echo "fixture failures: $failures"; exit "$failures"
