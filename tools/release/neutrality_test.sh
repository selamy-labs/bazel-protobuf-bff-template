#!/usr/bin/env bash
# Witness for neutrality.sh (spec 010 FR-005 to FR-007): a clean scratch repository, then planted defects in
# contents, names, history, metadata and refs, and the private mode's statuses and file boundaries.
#
#   neutrality_test.sh <neutrality.sh>
set -euo pipefail
S="$PWD/$1"; T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
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
echo "fixture failures: $failures"; exit "$failures"
