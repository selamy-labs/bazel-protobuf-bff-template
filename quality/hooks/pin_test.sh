#!/usr/bin/env bash
# Witness for pin.sh (spec 008 FR-002). Positive first, then each way two pins can stop being one.
set -euo pipefail
script="$PWD/$1"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
cd "$work"
sha=1111111111111111111111111111111111111111
other=2222222222222222222222222222222222222222
remote=file:///tmp/fixture/quality.git
fresh() {
  cat >MODULE.bazel <<M
module(name = "c")

bazel_dep(name = "bff_template_quality")
git_override(
    module_name = "bff_template_quality",
    commit = "$sha",
    remote = "$remote",
    strip_prefix = "quality",
)
M
  cat >prek.toml <<P
[[repos]]
repo = "$remote"
rev = "$sha"
hooks = [{ id = "pre-commit" }]
P
}
failures=0
expect() { # expect <name> <exit> <diagnostic> <mode>
  local rc=0 output
  output="$("$script" "$4" 2>&1)" || rc=$?
  if [[ "$rc" == "$2" && "$output" == *"$3"* ]]; then echo "ok   $1"; else echo "FAIL $1: exit $rc: $output"; failures=$((failures + 1)); fi
}

fresh
expect "one pin passes" 0 "" check
sed -i "s/^rev = .*/rev = \"$other\"/" prek.toml
expect "a differing rev" 1 "differs from" check
"$script" sync >/dev/null
expect "sync restores it" 0 "" check
grep -qx "rev = \"$sha\"" prek.toml || { echo "FAIL sync wrote no rev"; failures=$((failures + 1)); }
sed -i "s/$sha/$other/" MODULE.bazel
"$script" sync >/dev/null
expect "an upgrade is one value and a sync" 0 "" check
fresh
sed -i 's/commit = .*/commit = "main",/' MODULE.bazel
expect "a branch is not a pin" 1 "not a full SHA" check
fresh
sed -i 's/^rev = .*/rev = "v1"/' prek.toml
expect "a tag rev" 1 "differs from" check
fresh
sed -i '/^git_override(/,/^)/d' MODULE.bazel
printf 'local_path_override(\n    module_name = "bff_template_quality",\n    path = "../q",\n)\n' >>MODULE.bazel
expect "local_path_override" 1 "other than git_override" check
fresh
printf '\n[[repos]]\nrepo = "local"\n' >>prek.toml
expect "a local hook" 1 "local hook" check
fresh
sed -i 's#^repo = .*#repo = "file:///tmp/elsewhere.git"#' prek.toml
expect "another remote" 1 "0 repositories" check
exit "$failures"
