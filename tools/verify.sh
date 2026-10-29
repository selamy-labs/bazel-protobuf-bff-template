#!/usr/bin/env bash
# The canonical verifier (specs/002-hermetic-skeleton FR-004..FR-007). CI runs exactly this.
#
#   bazel run //:verify [-- --offline]
#   bazel run --config=weak-isolation //:verify -- --weak-isolation [--offline]
#
# --weak-isolation  explicit opt-in for hosts without namespace sandboxing; recorded in the receipt
# --offline         skip the networked Spec Kit provenance check (CI never passes this)
set -euo pipefail

root="${BUILD_WORKSPACE_DIRECTORY:-$(git rev-parse --show-toplevel)}"
cd "$root"

weak=false
offline=false
for arg in "$@"; do
  case "$arg" in
    --weak-isolation) weak=true ;;
    --offline) offline=true ;;
    *) echo "verify: unknown argument '$arg'" >&2; exit 2 ;;
  esac
done

bazel_flags=()
isolation=full
rerun="bazel run //:verify"
preflight_args=()
if $weak; then
  bazel_flags+=(--config=weak-isolation)
  isolation=weakened
  preflight_args+=(--allow-weak-isolation)
  rerun="bazel run --config=weak-isolation //:verify -- --weak-isolation"
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
results=()
status=0
check_timeout=900

run_check() { # run_check <id> <command...>; bounded, non-interactive, never aborts the run
  local id="$1" started rc=0
  shift
  started="$(date +%s)"
  timeout --kill-after=10 "$check_timeout" "$@" </dev/null >"$work/$id.log" 2>&1 || rc=$?
  local elapsed=$(($(date +%s) - started))
  if [[ $rc -eq 0 ]]; then
    printf 'PASS %-18s %4ss\n' "$id" "$elapsed"
    results+=("{\"check\":\"$id\",\"result\":\"PASSED\",\"seconds\":$elapsed}")
  else
    local state=FAILED
    [[ $rc -eq 124 || $rc -eq 137 ]] && state=TIMED_OUT
    printf '%s %-18s %4ss\n' "$state" "$id" "$elapsed"
    tail -n 50 "$work/$id.log" | sed 's/^/    /'
    echo "    (rerun: $rerun)"
    results+=("{\"check\":\"$id\",\"result\":\"$state\",\"seconds\":$elapsed}")
    status=1
  fi
}

if ! tools/bootstrap/preflight.sh "${preflight_args[@]}" >"$work/preflight.log" 2>&1; then
  cat "$work/preflight.log" >&2
  echo "verify: preflight failed; nothing was verified" >&2
  exit 1
fi

# Tools started with `bazel run` execute from their runfiles, so every path they see is absolute.
git ls-files -z --cached --others --exclude-standard >"$work/paths"
abs() { while IFS= read -r -d '' p; do if [[ -e "$root/$p" ]]; then printf '%s\0' "$root/$p"; fi; done; }
abs <"$work/paths" >"$work/abs"
# Authored files exclude the vendored Spec Kit scaffold, which is pinned by manifest instead
# (specs/002-hermetic-skeleton R4). Explicit file lists bypass the tools' own exclude settings.
grep -zv -e "^$root/\.specify/" -e "^$root/\.claude/skills/speckit-" "$work/abs" >"$work/authored" || true
tr '\0' '\n' <"$work/authored" >"$work/authored.nl"


run_check bazel-test bazel test "${bazel_flags[@]}" //... @bff_template_quality//...
run_check bzlmod-policy bazel run "${bazel_flags[@]}" //:workspace_policy -- "$work/paths"
run_check lock-fixture tools/policy/lock_fixture.sh
run_check suppression-ban bazel run "${bazel_flags[@]}" @bff_template_quality//checks/suppression:suppression-ban -- -root "$root" -paths "$work/paths"
run_check secrets bazel run "${bazel_flags[@]}" @bff_template_quality//tools:gitleaks -- dir --no-banner --redact --exit-code 1 --config "$root/.gitleaks.toml" "$root"
run_check typos bazel run "${bazel_flags[@]}" @bff_template_quality//tools:typos -- --config "$root/_typos.toml" --file-list "$work/authored.nl"
grep -zE '\.sh$' "$work/authored" >"$work/shell" || true
run_check shellcheck tools/checks/run_on_files.sh "$work/shell" bazel run "${bazel_flags[@]}" @bff_template_quality//tools:shellcheck --
grep -zE '(^|/)(BUILD|MODULE)\.bazel$|\.bzl$|(^|/)REPO\.bazel$' "$work/abs" >"$work/starlark" || true
run_check buildifier tools/checks/run_on_files.sh "$work/starlark" bazel run "${bazel_flags[@]}" @buildifier_prebuilt//:buildifier -- --mode=check --lint=warn
if [[ -d .github/workflows ]]; then
  run_check actionlint bazel run "${bazel_flags[@]}" @bff_template_quality//tools:actionlint -- -no-color "$root"/.github/workflows/*.yml
  run_check zizmor bazel run "${bazel_flags[@]}" @bff_template_quality//tools:zizmor -- --offline --persona=pedantic "$root/.github/workflows"
fi
if ! $offline; then
  run_check speckit-provenance tools/speckit/bootstrap.sh verify
fi

mkdir -p .verify
revision="$(git rev-parse HEAD)"
dirty=false
[[ -n "$(git status --porcelain)" ]] && dirty=true
joined="$(IFS=,; echo "${results[*]}")"
cat >.verify/receipt.json <<EOF
{
  "schema_version": 1,
  "revision": "$revision",
  "worktree_dirty": $dirty,
  "isolation": "$isolation",
  "offline": $offline,
  "result": "$([[ $status -eq 0 ]] && echo PASSED || echo FAILED)",
  "finished_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "checks": [$joined]
}
EOF
echo "verify: $([[ $status -eq 0 ]] && echo PASSED || echo FAILED) at $revision (isolation: $isolation$($dirty && echo ', worktree dirty'))"
exit "$status"
