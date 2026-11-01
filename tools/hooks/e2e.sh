#!/usr/bin/env bash
# The hooks with a real Bazel (spec 006 T014, FR-004, FR-009a, FR-010, FR-014). The `hook-e2e` check.
# In a copy of exactly the tested tree, with its own cache, so nothing here shares a server or a slot
# with the run that started it and only downloads are shared:
#   1. a rejected and a corrected real commit;
#   2. a push whose commit fails a test while the checkout holds the fix, then the corrected push;
#   3. a file that breaks the runner's own package: a bootstrap ERROR, not a verdict;
#   4. a deadline beside an independent `bazel build` on the same server: the runner stops only its own
#      client, and the other build finishes on the same server;
#   5. a pre-push deadline on the private output base: that server, and only it, stops.
#
#   tools/hooks/e2e.sh <NUL list of the tested tree> <full|weakened>
set -euo pipefail
list="$1"
isolation="$2"
root="$PWD"
bazel_in_root="$root/tools/bootstrap/bazel.sh"
binary="$("$bazel_in_root" --path)"
downloads="$("$bazel_in_root" info repository_cache </dev/null 2>/dev/null)"
user="${USER:-$(id -un)}"

work="$(mktemp -d)"
repo="$work/repo"
remote="$work/remote.git"
cleanup() {
  local base
  if [[ -x "$repo/tools/bootstrap/bazel.sh" ]]; then
    (cd "$repo" && tools/bootstrap/bazel.sh shutdown) </dev/null >/dev/null 2>&1 || true
    for base in "$XDG_CACHE_HOME"/bff-template/push/*; do
      if [[ -d "$base" ]]; then
        (cd "$repo" && tools/bootstrap/bazel.sh "--output_base=$base" shutdown) </dev/null >/dev/null 2>&1 || true
      fi
    done
  fi
  chmod -R u+w "${work:?}" 2>/dev/null || true
  rm -rf "${work:?}"
}
trap cleanup EXIT

export HOME="$work/home" XDG_CACHE_HOME="$work/home/.cache" TMPDIR="$work/tmp" PREK_HOME="$work/prek-home"
export GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL=a@example.com GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@example.com
mkdir -p "$TMPDIR" "$repo"
pinned="$XDG_CACHE_HOME/bff-template/bazel/$(basename "$(dirname "$binary")")"
mkdir -p "$pinned" "$XDG_CACHE_HOME/bazel/_bazel_$user/cache"
cp "$binary" "$pinned/bazel"
ln -s "$(dirname "$downloads")" "$XDG_CACHE_HOME/bazel/_bazel_$user/cache/repos"

die() {
  echo "hook-e2e: $1" >&2
  if [[ -s "$work/out" ]]; then tail -n 25 "$work/out" | cut -c1-200 >&2; fi
  exit 1
}
step() { # step <command...>: output in $work/out, exit code in $rc
  rc=0
  "$@" </dev/null >"$work/out" 2>&1 || rc=$?
}
in_repo() { git -C "$repo" "$@"; }
needs() { grep -qF -- "$1" "$work/out" || die "${2:-the output does not name '$1'}"; }
result() { "$YQ" -p=json -o=json -r ".checks[] | select(.check == \"$2\") | .result" "$1"; }

flags=()
install_flags=()
if [[ "$isolation" == weakened ]]; then
  flags+=(--config=weak-isolation)
  install_flags+=(-- --weak-isolation)
fi

# The tested tree, plus one package a push can break without touching policy inputs. Its producer is
# manual, so only the deadline case builds it.
(cd "$root" && tar --null -T "$list" -cf -) | tar -xf - -C "$repo"
mkdir -p "$repo/e2e_fixture"
cat >"$repo/e2e_fixture/BUILD.bazel" <<'FIXTURE'
load("@rules_shell//shell:sh_test.bzl", "sh_test")

sh_test(
    name = "value_test",
    size = "medium",
    srcs = ["value_test.sh"],
    data = [
        "slow.txt",
        "value.txt",
    ],
)

genrule(
    name = "producer",
    outs = ["producer.txt"],
    cmd = "sleep 40 && echo finished >$@",
    tags = ["manual"],
)
FIXTURE
cat >"$repo/e2e_fixture/value_test.sh" <<'FIXTURE'
#!/usr/bin/env bash
set -euo pipefail
if [[ -s e2e_fixture/slow.txt ]]; then sleep 287; fi
[[ "$(<e2e_fixture/value.txt)" == good* ]]
FIXTURE
chmod +x "$repo/e2e_fixture/value_test.sh"
echo good >"$repo/e2e_fixture/value.txt"
: >"$repo/e2e_fixture/slow.txt"

git init -q -b main "$repo"
in_repo add -A
in_repo commit -q --no-verify -m base
git init -q --bare -b main "$remote"
in_repo remote add origin "$remote"
in_repo push -q --no-verify origin main

step bash -c 'cd "$1" && shift && tools/bootstrap/bazel.sh run --noshow_progress "$@"' _ "$repo" "${flags[@]}" //tools/hooks:install "${install_flags[@]}"
[[ $rc -eq 0 ]] || die "the installation failed"
[[ "$(in_repo config bff-template.isolation)" == "$isolation" ]] || die "installed the wrong isolation"

# 1. A real commit, rejected and corrected.
printf 'token = "%s%s"\n' "ghp_" "aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0hJ3lN6" >"$repo/e2e_fixture/config.txt"
in_repo add e2e_fixture/config.txt
step in_repo commit -q -m secret
[[ $rc -ne 0 ]] || die "a staged secret was committed"
needs secrets
needs e2e_fixture/config.txt
needs "(rerun: "
in_repo rm -q --cached e2e_fixture/config.txt
rm -- "${repo:?}/e2e_fixture/config.txt"
echo bad >"$repo/e2e_fixture/value.txt"
in_repo add e2e_fixture/value.txt
step in_repo commit -q -m 'breaks the test'
[[ $rc -eq 0 ]] || die "a commit with clean content was rejected"
echo "ok   hook-e2e real commit rejected and corrected"

# 2. The commit is judged, not the checkout, which holds the fix in its index.
pushed="$(in_repo rev-parse HEAD)"
echo good >"$repo/e2e_fixture/value.txt"
in_repo add e2e_fixture/value.txt
receipt="$repo/.git/bff-template/receipts/pre-push.json"
step in_repo push -q origin main
[[ $rc -ne 0 ]] || die "a push whose commit fails a test was accepted"
needs "//e2e_fixture:value_test FAILED"
needs "(rerun: "
[[ "$(result "$receipt" tests)" == FAILED ]] || die "the receipt did not record the failure"
[[ "$("$YQ" -p=json -o=json -r .revision "$receipt")" == "$pushed" ]] || die "the receipt names another revision"
[[ "$(git -C "$remote" rev-parse main)" != "$pushed" ]] || die "the failing commit reached the remote"
step in_repo commit -q -m 'fixes the test'
[[ $rc -eq 0 ]] || die "the fix was not committed"
pushed="$(in_repo rev-parse HEAD)"
step in_repo push -q origin main
[[ $rc -eq 0 ]] || die "the corrected push failed"
[[ "$(result "$receipt" tests)" == PASSED ]] || die "the receipt did not record the pass"
[[ "$("$YQ" -p=json -o=json -r .revision "$receipt")" == "$pushed" ]] || die "the receipt names another revision"
[[ "$("$YQ" -p=json -o=json -r .isolation "$receipt")" == "$isolation" ]] || die "the receipt lost the isolation label"
[[ "$(git -C "$remote" rev-parse main)" == "$pushed" ]] || die "the corrected commit did not arrive"
echo "ok   hook-e2e failing push and corrected push"

# 3. An untracked bad file in an ordinary package changes nothing. One that breaks the runner's own
# package stops the hook before any check, and says so.
printf 'func  bad\n' >"$repo/e2e_fixture/untracked.go"
echo 'more' >"$repo/e2e_fixture/note.txt"
in_repo add e2e_fixture/note.txt
step in_repo commit -q -m 'beside an untracked file'
[[ $rc -eq 0 ]] || die "an untracked file in an ordinary package changed the verdict"
rm -- "${repo:?}/e2e_fixture/untracked.go"
# An empty untracked BUILD file makes a package of a directory the root package names files in, and
# the root package holds //:quality.
boundary="$repo/tools/checks/BUILD.bazel"
: >"$boundary"
echo 'again' >"$repo/e2e_fixture/note.txt"
in_repo add e2e_fixture/note.txt
step in_repo commit -q -m bootstrap
[[ $rc -ne 0 ]] || die "a runner that cannot build passed"
needs "bootstrap ERROR"
needs "no check ran"
if grep -qE '^ *(FAILED|PASS|SKIP) ' "$work/out"; then die "a bootstrap failure reported a check"; fi
rm -- "${boundary:?}"
step in_repo commit -q -m 'after the repair'
[[ $rc -eq 0 ]] || die "the commit after the repair failed"
echo "ok   hook-e2e a broken bootstrap is an ERROR and an untracked file is not read"

# 4. A deadline beside an independent producer on the same server.
step bash -c 'cd "$1" && shift && tools/bootstrap/bazel.sh run --noshow_progress "--script_path=$1" "${@:2}"' _ "$repo" "$work/runner" "${flags[@]}" //:quality
[[ $rc -eq 0 ]] || die "the runner did not build"
base="$(cd "$repo" && tools/bootstrap/bazel.sh info output_base </dev/null 2>/dev/null)"
server="$(<"$base/server/server.pid.txt")"
cat >"$work/one-check.textproto" <<'FIXTURE'
version: "e2e"
vendored_manifest: "tools/speckit/scaffold.sha256"
fixture_manifest: "quality/checks/suppression/testdata/fixtures.sha256"
checks {
  id: "tests"
  stages: [STAGE_FULL]
  classification: CLASSIFICATION_GUARANTEE
  kind: KIND_BAZEL
  timeout_seconds: 120
  targets: ["//e2e_fixture:value_test"]
  witness: "//:verify_body_test"
}
FIXTURE
(cd "$repo" && exec tools/bootstrap/bazel.sh build "${flags[@]}" //e2e_fixture:producer) </dev/null >"$work/producer.log" 2>&1 &
producer=$!
for _ in $(seq 1 100); do
  if grep -qF 'Analyzed target' "$work/producer.log"; then break; fi
  sleep 0.2
done
grep -qF 'Analyzed target' "$work/producer.log" || die "the producer did not start"
sleep 2
step "$work/runner" --stage=full --deadline=6s "--registry=$work/one-check.textproto" "--isolation=$isolation"
kill -0 "$producer" 2>/dev/null || die "control failed: the producer ended before the deadline, so this proves nothing"
[[ $rc -eq 10 ]] || die "a deadline exited $rc, not 10"
needs TIMED_OUT
needs "a shared server is left running"
kill -0 "$server" 2>/dev/null || die "the deadline took the shared server with it"
producer_rc=0
wait "$producer" || producer_rc=$?
[[ $producer_rc -eq 0 ]] || { cp "$work/producer.log" "$work/out"; die "the independent producer exited $producer_rc"; }
[[ "$(<"$base/server/server.pid.txt")" == "$server" ]] || die "the server changed under the producer"
kill -0 "$server" 2>/dev/null || die "the server did not outlive the producer"
echo "ok   hook-e2e a deadline left the independent producer and its server running"

# 5. On the private output base, where the lock gives this run exclusive use, a deadline stops the server.
from="$(in_repo rev-parse HEAD)"
echo slow >"$repo/e2e_fixture/slow.txt"
in_repo add e2e_fixture/slow.txt
in_repo commit -q --no-verify -m 'a slow test'
to="$(in_repo rev-parse HEAD)"
owned=("$XDG_CACHE_HOME"/bff-template/push/*/server/server.pid.txt)
[[ ${#owned[@]} -eq 1 && -f "${owned[0]}" ]] || die "expected one private server, found ${#owned[@]}"
private="$(<"${owned[0]}")"
kill -0 "$private" 2>/dev/null || die "control failed: the private server was not running, so this proves nothing"
step env "BUILD_WORKSPACE_DIRECTORY=$repo" "$work/runner" --stage=pre-push --deadline=45s "--to=$to" "--from=$from" "--isolation=$isolation"
[[ $rc -eq 10 ]] || die "a pre-push deadline exited $rc, not 10"
needs "pre-push deadline passed"
if kill -0 "$private" 2>/dev/null; then die "the private server outlived the deadline"; fi
kill -0 "$server" 2>/dev/null || die "the checkout's server was stopped too"
if pgrep -f 'sleep 287' >/dev/null; then die "the slow test outlived the deadline"; fi
echo "ok   hook-e2e a pre-push deadline stopped only the private server"
