#!/usr/bin/env bash
# The hook contract's fixtures (contracts/hook-stages.md, T043, spec 006 T013). Each runs in a temporary
# repository with real Git, the real pinned prek, the real stage entry, and the real registry, runner
# and tools. Only Bazel is replaced (fixture_bazel.sh), so a rejection here comes from a real check. A
# rejection earns credit only if it names the check, the path and the rerun command, and its corrected
# change then passes. tools/hooks/e2e.sh repeats what needs a real Bazel.
#
#   tools/hooks/fixtures_test.sh <runner> <adapter> <NAME=path>...
#
# Every removal below names a path inside $work, which this script created, through a checked expansion.
set -euo pipefail
runner="$PWD/$1"
adapter="$PWD/$2"
shift 2
source_root="$PWD"
registry="$PWD/${REGISTRY:?}"
tools=()
pinned_prek=""
yq=""
for pair in "$@"; do
  tools+=("--tool=${pair%%=*}=$PWD/${pair#*=}")
  case "${pair%%=*}" in
    PREK) pinned_prek="$PWD/${pair#*=}" ;;
    YQ) yq="$PWD/${pair#*=}" ;;
  esac
done
work="$(mktemp -d)"
trap 'chmod -R u+w "${work:?}" 2>/dev/null; rm -rf "${work:?}"' EXIT
export HOME="$work/home" XDG_CACHE_HOME="$work/cache" PREK_HOME="$work/prek-home" TMPDIR="$work"
export PATH=/usr/bin:/bin GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null
export GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL=a@example.com GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@example.com
mkdir -p "$HOME"
failures=0
cases=0
rc=0
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }
pass() { cases=$((cases + 1)); echo "ok   $1"; }
show() { sed 's/^/    /' "$work/out" | head -"${1:-30}" >&2; }
# Assembled here so this file holds no token of its own.
token() { printf '%s%s' "ghp_" "aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0hJ3lN6"; }

# new_repo <dir> [prek]: one commit and the hooks installed. <dir> may already be initialized.
new_repo() {
  local dir="$1" prek="${2:-$pinned_prek}"
  mkdir -p "$dir/src" "$dir/api/a/v1" "$dir/tools/hooks" "$dir/tools/bootstrap" "$dir/tools/speckit" \
    "$dir/quality/checks/suppression/testdata"
  # Only this directory counts: the test's temporary directory may sit inside another repository.
  if [[ ! -e "$dir/.git" ]]; then git -C "$dir" init -q -b main; fi
  cp "$source_root/prek.toml" "$source_root/.gitleaks.toml" "$source_root/_typos.toml" "$dir/"
  cp "$source_root/tools/hooks/stage.sh" "$dir/tools/hooks/"
  cp "$source_root/tools/hooks/fixture_bazel.sh" "$dir/tools/bootstrap/bazel.sh"
  printf '%s\n' "$runner" "$registry" "$adapter" "$source_root" "${tools[@]}" >"$dir/tools/bootstrap/bazel.config"
  echo vendored >"$dir/tools/speckit/vendored.txt"
  echo fixture >"$dir/quality/checks/suppression/testdata/fixture.txt"
  (cd "$dir" && sha256sum tools/speckit/vendored.txt >tools/speckit/scaffold.sha256)
  (cd "$dir" && sha256sum quality/checks/suppression/testdata/fixture.txt >quality/checks/suppression/testdata/fixtures.sha256)
  printf 'package src\n\nfunc Value() int { return 1 }\n' >"$dir/src/app.go"
  printf 'syntax = "proto3";\n\npackage a.v1;\n\nmessage Note {\n  string title = 1;\n}\n' >"$dir/api/a/v1/note.proto"
  git -C "$dir" add -A
  git -C "$dir" commit -q -m base
  (cd "$dir" && BUILD_WORKSPACE_DIRECTORY="$dir" bash "$source_root/tools/hooks/install.sh" "$prek" >/dev/null)
}

commit() { # commit <dir> <git commit arguments...>: output in $work/out, exit code in $rc
  local dir="$1"
  shift
  rc=0
  git -C "$dir" commit "$@" </dev/null >"$work/out" 2>&1 || rc=$?
}
push() { # push <dir> <git push arguments...>: output in $work/out, exit code in $rc
  local dir="$1"
  shift
  rc=0
  git -C "$dir" push "$@" </dev/null >"$work/out" 2>&1 || rc=$?
}
rejected() { # rejected <label> <text...>: the last step failed and named each text and the rerun
  local label="$1" needle
  shift
  if [[ $rc -eq 0 ]]; then fail "$label: accepted"; show; return; fi
  for needle in "$@" "(rerun: "; do
    if ! grep -qF -- "$needle" "$work/out"; then fail "$label: no '$needle' in the output"; show; return; fi
  done
  pass "$label"
}
accepted() { # accepted <label>: the last step passed
  if [[ $rc -ne 0 ]]; then fail "$1: rejected ($rc)"; show; else pass "$1"; fi
}
state_of() { # state_of <receipt> <check>: the check's recorded result
  "$yq" -p=json -o=json -r ".checks[] | select(.check == \"$2\") | .result" "$1"
}

# --- Installation: a foreign hook, and a second install.
r="$work/repo with spaces"
mkdir -p "$r"
git -C "$r" init -q -b main
printf '#!/bin/sh\necho ran >>"%s/foreign"\n' "$work" >"$r/.git/hooks/pre-commit"
chmod +x "$r/.git/hooks/pre-commit"
new_repo "$r"
: >"$work/foreign" # The base commit ran it before the install; only what follows counts.
hooks=("$r/.git/hooks/pre-commit" "$r/.git/hooks/pre-push" "$r/.git/hooks/pre-commit.legacy")
before="$(sha256sum "${hooks[@]}")"
(cd "$r" && BUILD_WORKSPACE_DIRECTORY="$r" bash "$source_root/tools/hooks/install.sh" "$pinned_prek" >/dev/null) ||
  fail "a second install failed"
if [[ "$(sha256sum "${hooks[@]}")" == "$before" ]]; then
  pass "a second install changes nothing"
else
  fail "a second install changed the hooks"
fi
if grep -qF "$r/.git/bff-template/bin/prek" "$r/.git/hooks/pre-commit"; then
  pass "the hook names the pinned copy beside the Git data"
else
  fail "the hook names another prek"
fi
if [[ "$(git -C "$r" config bff-template.isolation)" == full ]]; then
  pass "installation defaults to full isolation"
else
  fail "installation did not record full isolation"
fi

echo clean >"$r/src/note.txt"
git -C "$r" add src/note.txt
commit "$r" -q -m clean
accepted "a clean staged commit"
if [[ -s "$work/foreign" ]]; then pass "a foreign hook still runs"; else fail "the foreign hook did not run"; fi

# --- Commit failures, each with its corrected change.
printf 'token = "%s"\n' "$(token)" >"$r/src/config.txt"
git -C "$r" add src/config.txt
commit "$r" -q -m secret
rejected "a staged secret" "secrets" "src/config.txt"
if grep -qF "$(token)" "$work/out"; then fail "the secret reached the output"; fi
printf 'token = read_from_environment()\n' >"$r/src/config.txt"
git -C "$r" add src/config.txt
commit "$r" -q -m corrected
accepted "the corrected secret"

printf 'package src\n\nvar x = 1 //%s\n' "nolint" >"$r/src/lint.go"
git -C "$r" add src/lint.go
commit "$r" -q -m suppression
rejected "a banned suppression" "suppression-ban" "src/lint.go" "[S001]"
printf 'package src\n\nvar x = 1\n' >"$r/src/lint.go"
git -C "$r" add src/lint.go
commit "$r" -q -m corrected
accepted "the corrected suppression"

printf 'syntax = "proto3";\nmessage {\n' >"$r/api/a/v1/note2.proto"
git -C "$r" add api/a/v1/note2.proto
commit "$r" -q -m proto
rejected "a malformed proto" "format-proto" "note2.proto"
printf 'syntax = "proto3";\n\npackage a.v1;\n\nmessage Other {\n  string title = 1;\n}\n' >"$r/api/a/v1/note2.proto"
git -C "$r" add api/a/v1/note2.proto
commit "$r" -q -m corrected
accepted "the corrected proto"

# --- Configuration.
cp "$r/prek.toml" "$work/prek.toml"
printf 'not toml [\n' >"$r/prek.toml"
git -C "$r" add prek.toml
echo changed >"$r/src/note.txt"
git -C "$r" add src/note.txt
commit "$r" -q -m malformed
if [[ $rc -ne 0 ]] && grep -qiE 'parse|invalid|toml' "$work/out"; then
  pass "a malformed prek.toml refuses the commit"
else
  fail "a malformed prek.toml: exit $rc"
  show
fi
cp "$work/prek.toml" "$r/prek.toml"
git -C "$r" add prek.toml
printf 'checks {\n' >"$r/BAD_REGISTRY"
commit "$r" -q -m registry
if [[ $rc -ne 0 ]] && grep -qF 'configuration error' "$work/out"; then
  pass "a malformed registry refuses the commit"
else
  fail "a malformed registry: exit $rc"
  show
fi
rm -- "${r:?}/BAD_REGISTRY"
commit "$r" -q -m restored
accepted "the commit after the registry is restored"

# --- Bootstrap is not a verdict, and an untracked file in an ordinary package is not read.
printf 'token = "%s"\nfunc  bad\n' "$(token)" >"$r/src/untracked.go"
sum="$(sha256sum "$r/src/untracked.go")"
echo unrelated >"$r/src/note.txt"
git -C "$r" add src/note.txt
commit "$r" -q -m unrelated
accepted "an untracked bad file in the same package does not change the verdict"
[[ "$(sha256sum "$r/src/untracked.go")" == "$sum" ]] || fail "the untracked file changed"

touch "$r/BOOTSTRAP_BROKEN"
echo again >"$r/src/note.txt"
git -C "$r" add src/note.txt
commit "$r" -q -m bootstrap
if [[ $rc -eq 0 ]]; then
  fail "a runner that cannot start passed"
elif ! grep -qF 'bootstrap ERROR' "$work/out" || ! grep -qF 'no check ran' "$work/out"; then
  fail "a bootstrap failure was not named"
  show
elif grep -qE '^(FAILED|PASSED|quality:) ' "$work/out"; then
  fail "a bootstrap failure reported a check"
  show
else
  pass "a broken bootstrap is an ERROR that names no check"
fi
rm -- "${r:?}/BOOTSTRAP_BROKEN" "${r:?}/src/untracked.go"
commit "$r" -q -m restored
accepted "the commit after bootstrap is repaired"

# --- Partial staging, 20 runs: the verdict follows the index; bytes and index stay (SC-002).
partial_ok=true
for i in $(seq 1 20); do
  printf 'package src\n\nfunc  Value() int { return %d }\n' "$i" >"$r/src/app.go"
  git -C "$r" add src/app.go
  printf 'package src\n\n// unstaged edit %d\nfunc Value() int { return 1 }\n' "$i" >"$r/src/app.go"
  sum="$(sha256sum "$r/src/app.go")"
  index="$(git -C "$r" ls-files --stage src/app.go)"
  commit "$r" -q -m partial
  if [[ $rc -eq 0 ]] || ! grep -qF format-go "$work/out"; then
    partial_ok=false
    fail "run $i: a staged format defect passed, or failed for another reason"
    show
  fi
  [[ "$(sha256sum "$r/src/app.go")" == "$sum" ]] || { partial_ok=false; fail "run $i: unstaged bytes changed"; }
  [[ "$(git -C "$r" ls-files --stage src/app.go)" == "$index" ]] || { partial_ok=false; fail "run $i: the index changed"; }
done
if $partial_ok; then pass "partial staging, 20 runs"; fi
git -C "$r" checkout -q HEAD -- src/app.go

printf 'package src\n\nfunc Value() int { return 2 }\n' >"$r/src/app.go"
git -C "$r" add src/app.go
printf 'package src\n\nfunc  Value() int { return 2 }\n' >"$r/src/app.go"
sum="$(sha256sum "$r/src/app.go")"
commit "$r" -q -m clean-staged
accepted "a clean staged blob passes beside a bad unstaged edit"
[[ "$(sha256sum "$r/src/app.go")" == "$sum" ]] || fail "the unstaged edit changed"

# --- The index Git names: `commit -a` and `commit <path>` commit a temporary one.
commit "$r" -q -a -m all
rejected "git commit -a judges what it is about to commit" "format-go" "src/app.go"
git -C "$r" checkout -q HEAD -- src/app.go
printf 'package src\n\nfunc  Other() {}\n' >"$r/src/other.go"
git -C "$r" add src/other.go
echo fine >"$r/src/path.txt"
git -C "$r" add src/path.txt
commit "$r" -q -m path src/path.txt
accepted "git commit <path> judges only that path"
git -C "$r" rm -q --cached src/other.go
rm -- "${r:?}/src/other.go"

# --- Paths.
git -C "$r" mv src/note.txt "src/re named.txt"
commit "$r" -q -m rename
accepted "a rename"
git -C "$r" rm -q "src/re named.txt"
commit "$r" -q -m delete
accepted "a deletion-only commit"
printf 'token = "%s"\n' "$(token)" >"$r/src/with space.txt"
git -C "$r" add "src/with space.txt"
commit "$r" -q -m space
rejected "a filename with spaces" "secrets" "with space.txt"
git -C "$r" rm -q --cached "src/with space.txt"
rm -- "${r:?}/src/with space.txt"
new=$'src/new\nline.txt'
printf 'token = "%s"\n' "$(token)" >"$r/$new"
git -C "$r" add "$new"
commit "$r" -q -m newline
rejected "a filename with a newline" "secrets"
git -C "$r" rm -q --cached "$new"
rm -- "${r:?}/${new:?}"

# --- Scale: a whole-project check runs once, over every file.
mkdir -p "$r/batch"
for i in $(seq 1 500); do echo "file $i" >"$r/batch/f$i.txt"; done
git -C "$r" add batch
commit "$r" -q -m batch
accepted "a 500-file batch"
receipt="$r/.git/bff-template/receipts/pre-commit.json"
runs="$("$yq" -p=json -o=json -r '[.checks[] | select(.check == "secrets")] | length' "$receipt")"
inputs="$("$yq" -p=json -o=json -r '.checks[] | select(.check == "secrets") | .inputs' "$receipt")"
if [[ "$runs" == 1 && "$inputs" == 500 ]]; then
  pass "secrets ran once, over all 500 files"
else
  fail "secrets ran $runs times over $inputs files"
fi

# --- A linked worktree.
git -C "$r" worktree add -q -b linked "$work/linked"
printf 'package src\n\nfunc  Linked() {}\n' >"$work/linked/src/linked.go"
git -C "$work/linked" add src/linked.go
commit "$work/linked" -q -m bad
rejected "a commit in a linked worktree" "format-go" "src/linked.go"
printf 'package src\n\nfunc Linked() {}\n' >"$work/linked/src/linked.go"
git -C "$work/linked" add src/linked.go
commit "$work/linked" -q -m good
accepted "the corrected commit in a linked worktree"

# --- core.hooksPath.
h="$work/hookspath"
mkdir -p "$h"
git -C "$h" init -q -b main
git -C "$h" config core.hooksPath .myhooks
new_repo "$h"
if [[ -x "$h/.myhooks/pre-commit" ]]; then pass "installation honors core.hooksPath"; else fail "core.hooksPath was ignored"; fi
printf 'package src\n\nfunc  Bad() {}\n' >"$h/src/bad.go"
git -C "$h" add src/bad.go
commit "$h" -q -m bad
rejected "hooks under core.hooksPath run" "format-go" "src/bad.go"

# --- The pin survives losing the copy it was installed from, with no prek on PATH.
pin="$work/pin"
mkdir -p "$pin"
cp "$(realpath "$pinned_prek")" "$work/prek-to-lose"
new_repo "$pin" "$work/prek-to-lose"
rm -- "${work:?}/prek-to-lose"
printf 'package src\n\nfunc  Pin() {}\n' >"$pin/src/pin.go"
git -C "$pin" add src/pin.go
commit "$pin" -q -m bad
rejected "hooks still judge after the installed-from binary is gone" "format-go" "src/pin.go"

# The limit docs/quality/hooks.md states: with the copy deleted too, prek's shim takes any prek on PATH.
mkdir -p "$work/fake"
printf '#!/bin/sh\necho not-the-pin\nexit 0\n' >"$work/fake/prek"
chmod +x "$work/fake/prek"
rm -- "${pin:?}/.git/bff-template/bin/prek"
rc=0
PATH="$work/fake:$PATH" git -C "$pin" commit -q -m limit </dev/null >"$work/out" 2>&1 || rc=$?
if [[ $rc -eq 0 ]] && grep -qF not-the-pin "$work/out"; then
  pass "the documented limit: a deleted pin falls back to PATH"
else
  fail "the PATH fallback changed; correct docs/quality/hooks.md (exit $rc)"
  show
fi

# --- Push: the pushed commit is judged, not the working tree.
p="$work/push"
remote="$work/remote.git"
git init -q --bare -b main "$remote"
mkdir -p "$p"
new_repo "$p"
git -C "$p" remote add origin "$remote"
git -C "$p" push -q --no-verify origin main

echo x >"$p/BROKEN"
git -C "$p" add BROKEN
commit "$p" -q -m 'breaks a test'
accepted "pre-commit does not run tests"
# Staged, because prek stashes unstaged changes at pre-push too, which would restore the defect to
# the checkout and leave this case unable to tell the pushed commit from the working tree.
git -C "$p" rm -q BROKEN # The checkout now holds the fix; the commit does not.
push "$p" -q origin main
rejected "a failing push, with a clean working tree" "tests //src:unit_test FAILED"
if [[ "$(git -C "$remote" rev-parse main)" == "$(git -C "$p" rev-parse HEAD)" ]]; then
  fail "the failing commit reached the remote"
fi
commit "$p" -q -m 'fixes the test'
push "$p" -q origin main
accepted "the corrected push"
[[ "$(git -C "$remote" rev-parse main)" == "$(git -C "$p" rev-parse HEAD)" ]] || fail "the corrected commit did not arrive"

echo x >"$p/BROKEN" # Now the checkout is bad and the commit is fine.
echo y >"$p/src/more.txt"
git -C "$p" add src/more.txt
commit "$p" -q -m more
push "$p" -q origin main
accepted "an untracked file in the checkout cannot fail a clean pushed commit"
rm -- "${p:?}/BROKEN"
if grep -qF 'escalated to full' "$work/out"; then fail "an ordinary push escalated"; else pass "an ordinary push stays pre-push"; fi
worktree="$p/.git/bff-template/push-worktree"
[[ "$(git -C "$worktree" rev-parse HEAD)" == "$(git -C "$p" rev-parse HEAD)" ]] || fail "the worktree is not at the pushed commit"
[[ "$(git -C "$p" symbolic-ref --short HEAD)" == main ]] || fail "the push moved the checkout"
[[ "$(state_of "$p/.git/bff-template/receipts/pre-push.json" tests)" == PASSED ]] || fail "the receipt lost the pushed verdict"

# The contract's limit, kept visible: prek evaluates only the first ref of a multi-ref push.
git -C "$p" checkout -q -b other
echo x >"$p/BROKEN"
git -C "$p" add BROKEN
git -C "$p" commit -q --no-verify -m 'breaks a test on the second ref'
git -C "$p" checkout -q main
echo z >"$p/src/z.txt"
git -C "$p" add src/z.txt
commit "$p" -q -m 'first ref'
push "$p" -q origin main other
accepted "the documented limit: a failure on a second pushed ref is not seen locally"

# --- Bypass is real, and the full policy still catches it.
printf 'package src\n\nfunc  Skipped() {}\n' >"$p/src/skipped.go"
git -C "$p" add src/skipped.go
rc=0
SKIP=pre-commit git -C "$p" commit -q -m skip </dev/null >"$work/out" 2>&1 || rc=$?
accepted "SKIP passes a staged defect"
printf 'package src\n\nfunc Skipped() {}\n' >"$p/src/skipped.go"
git -C "$p" add src/skipped.go
git -C "$p" commit -q --no-verify -m 'corrects it'
printf 'package src\n\nfunc  Skipped() {}\n' >"$p/src/skipped.go"
git -C "$p" add src/skipped.go
rc=0
PREK_SKIP=pre-commit git -C "$p" commit -q -m skip </dev/null >"$work/out" 2>&1 || rc=$?
accepted "PREK_SKIP passes a staged defect"
printf 'package src\n\nfunc Skipped() {}\n' >"$p/src/skipped.go"
git -C "$p" add src/skipped.go
git -C "$p" commit -q --no-verify -m 'corrects it'

printf 'token = "%s"\n' "$(token)" >"$p/src/bypassed.txt"
git -C "$p" add src/bypassed.txt
commit "$p" -q --no-verify -m bypassed
accepted "git commit --no-verify passes a secret"
push "$p" -q --no-verify origin main
accepted "git push --no-verify delivers it"
script="$work/full-runner"
(cd "$p" && tools/bootstrap/bazel.sh run "--script_path=$script" //:quality)
rc=0
"$script" --stage=full --deadline=120s "--receipt=$work/full.json" </dev/null >"$work/out" 2>&1 || rc=$?
rejected "the full policy catches the bypassed secret" "secrets" "src/bypassed.txt"
[[ "$(state_of "$work/full.json" secrets)" == FAILED ]] || fail "full did not record secrets as FAILED"
# A hosted run is the same policy, one leg per job, so that leg must catch it too. It runs with no base
# here, and the secret is still a finding beside the refusal.
rc=0
"$script" --stage=full --shard=policy --deadline=120s "--receipt=$work/leg.json" </dev/null >"$work/out" 2>&1 || rc=$?
rejected "the CI leg catches the bypassed secret" "secrets" "src/bypassed.txt"
[[ "$(state_of "$work/leg.json" secrets)" == FAILED ]] || fail "the policy leg did not record secrets as FAILED"

# --- The policy base at pre-push. prek gives a new branch its fork point as --from, so an ordinary first
# push stays narrow and needs no base. A policy change, or a push with no base in reach, widens to `full`,
# and then only the declared setting supplies one. A small registry keeps `full` passable here, and its
# check needs a base as the real one does.
t="$work/topic"
mkdir -p "$t"
new_repo "$t"
mkdir -p "$t/quality/checks/coverage/cmd/floors"
echo 'package main' >"$t/quality/checks/coverage/cmd/floors/main.go"
echo '{}' >"$t/quality/coverage_floors.json"
cat >"$work/base.textproto" <<'REGISTRY'
version: "0.0.0"
vendored_manifest: "tools/speckit/scaffold.sha256"
fixture_manifest: "quality/checks/suppression/testdata/fixtures.sha256"
escalate: "^quality/"
checks {
  id: "compare"
  stages: [STAGE_FULL]
  classification: CLASSIFICATION_GUARANTEE
  kind: KIND_NATIVE
  timeout_seconds: 30
  needs_base: true
  command: ["sh", "-c", "echo compared with {base}"]
  witness: "//:test"
}
REGISTRY
cp "$work/base.textproto" "$t/quality/registry.textproto"
sed -i "2s|.*|$work/base.textproto|" "$t/tools/bootstrap/bazel.config"
git -C "$t" add -A
git -C "$t" commit -q --no-verify -m evaluator
topic_remote="$work/topic.git"
git init -q --bare "$topic_remote"
git -C "$t" remote add origin "$topic_remote"
git -C "$t" push -q --no-verify origin main
reviewed="$(git -C "$t" rev-parse main)"
receipt="$t/.git/bff-template/receipts/pre-push.json"
of_receipt() { "$yq" -p=json -o=json -r ".$1" "$receipt"; }

git -C "$t" switch -q -c ordinary
echo more >"$t/more.txt"
git -C "$t" add more.txt
git -C "$t" commit -q --no-verify -m ordinary
push "$t" -q origin ordinary
accepted "an ordinary first push of a topic branch needs no base"
[[ "$(of_receipt stage)" == pre-push ]] || fail "an ordinary first push widened"

# A setting the push never needs cannot block it.
git -C "$t" config bff-template.policy-base no-such-ref
echo more >>"$t/more.txt"
git -C "$t" add more.txt
git -C "$t" commit -q --no-verify -m ordinary-again
push "$t" -q origin ordinary
accepted "an ordinary push is not judged by a setting it does not use"
[[ "$(of_receipt base_refusal)" == null ]] || fail "an ordinary push recorded a refusal"
git -C "$t" config --unset bff-template.policy-base

git -C "$t" switch -q -c policy main
echo '{ }' >"$t/quality/coverage_floors.json"
git -C "$t" add -A
git -C "$t" commit -q --no-verify -m policy
push "$t" -q origin policy
rejected "a first push that needs a base, with none declared, is refused" "no policy base was named" "--policy-base=<commit>"
grep -qF 'exit code: 11' "$work/out" || fail "the refusal was not exit 11"
if git -C "$topic_remote" rev-parse -q --verify refs/heads/policy >/dev/null; then fail "the refused push arrived"; fi
if grep -qF 'compared with' "$work/out"; then fail "something stood in for the base"; fi

rc=0
(cd "$t" && BUILD_WORKSPACE_DIRECTORY="$t" bash "$source_root/tools/hooks/install.sh" "$pinned_prek" --policy-base= >/dev/null 2>&1) || rc=$?
[[ $rc -eq 2 ]] || fail "an empty --policy-base: exit $rc, want 2"
[[ -z "$(git -C "$t" config --get bff-template.policy-base || true)" ]] || fail "an empty value was stored"
(cd "$t" && BUILD_WORKSPACE_DIRECTORY="$t" bash "$source_root/tools/hooks/install.sh" "$pinned_prek" --policy-base=main >/dev/null) ||
  fail "installing a policy base failed"
[[ "$(git -C "$t" config --get bff-template.policy-base)" == main ]] || fail "the setting holds what was given, unresolved"

push "$t" -q origin policy
accepted "a first push of a policy change, with a declared base"
git -C "$topic_remote" rev-parse -q --verify refs/heads/policy >/dev/null || fail "the accepted push did not arrive"
[[ "$(of_receipt stage)" == full ]] || fail "a policy change must widen to full"
[[ "$(of_receipt base_revision)" == "$reviewed" ]] || fail "the receipt lost the resolved base"
[[ "$(of_receipt base_source)" == hook-config ]] || fail "the receipt lost how the base was chosen"

# A zero --from, as prek passes when no remote commit is in reach, widens too and takes the same base.
zero="$(printf '0%.0s' {1..40})"
pushed="$(git -C "$t" rev-parse HEAD)"
rc=0
(cd "$t" && PRE_COMMIT_FROM_REF="$zero" PRE_COMMIT_TO_REF="$pushed" tools/hooks/stage.sh pre-push) </dev/null >"$work/out" 2>&1 || rc=$?
accepted "a zero --from with a declared base"
[[ "$(of_receipt stage)" == full ]] || fail "a zero --from must widen to full"
[[ "$(of_receipt base_source)" == hook-config ]] || fail "a zero --from lost the declared base"

git -C "$t" config --unset bff-template.policy-base
rc=0
(cd "$t" && PRE_COMMIT_FROM_REF="$zero" PRE_COMMIT_TO_REF="$pushed" tools/hooks/stage.sh pre-push) </dev/null >"$work/out" 2>&1 || rc=$?
[[ $rc -eq 11 ]] || fail "a zero --from with no declared base: exit $rc, want 11"
grep -qF 'no policy base was named' "$work/out" || fail "the zero --from refusal named no cause"

git -C "$t" config bff-template.policy-base HEAD
echo '{  }' >"$t/quality/coverage_floors.json"
git -C "$t" add -A
git -C "$t" commit -q --no-verify -m again
push "$t" -q origin policy
rejected "a declared base that resolves to the pushed commit is refused" "a commit cannot be its own base"

# --- A policy input widens the push, which cannot then pass here: the verdict is the full stage's.
git -C "$p" rm -q src/bypassed.txt
git -C "$p" commit -q --no-verify -m 'removes it'
echo '# a comment' >>"$p/prek.toml"
git -C "$p" add prek.toml
git -C "$p" commit -q --no-verify -m 'changes policy'
push "$p" -q origin main
if grep -qF 'escalated to full' "$work/out"; then pass "a policy change escalates the push"; else fail "a policy change did not escalate"; show; fi

# --- The document states what these cases showed. A limit that changes must change the document.
for statement in "git commit --no-verify" "git push --no-verify" "SKIP=pre-commit" "PREK_SKIP=pre-commit" \
  "Only the first pushed ref is judged" "A deleted pin falls back to \`PATH\`" "A foreign hook needs a shebang" \
  "bootstrap ERROR" "at most the newest 5"; do
  if grep -qF -- "$statement" "$source_root/${HOOKS_DOC:?}"; then
    pass "the hooks document states: $statement"
  else
    fail "the hooks document lost: $statement"
  fi
done

echo "cases=$cases failures=$failures"
if [[ $cases -lt 50 ]]; then
  echo "FAIL expected at least 50 cases, ran $cases" >&2
  exit 1
fi
exit "$failures"
