#!/usr/bin/env bash
# The body of tools/verify.sh, run in its scrubbed environment. Do not call it directly.
set -euo pipefail
root="$1"
shift
cd "$root"

weak=false
offline=false
bootstrap_only=false
for arg in "$@"; do
  case "$arg" in
    --weak-isolation) weak=true ;;
    --offline) offline=true ;;
    --bootstrap-only) bootstrap_only=true ;;
    *) echo "verify: unknown argument '$arg'" >&2; exit 2 ;;
  esac
done

bazel=("$root/tools/bootstrap/bazel.sh")
bazel_flags=()
isolation=full
rerun="tools/verify.sh"
preflight_args=()
if $weak; then
  bazel_flags+=(--config=weak-isolation)
  isolation=weakened
  rerun="tools/verify.sh --weak-isolation"
  preflight_args+=(--allow-weak-isolation)
fi
if $offline; then
  bazel_flags+=(--repository_disable_download)
  rerun+=" --offline"
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
results=()
status=0
check_timeout=900

record() { # record <id> <state> <seconds>
  results+=("{\"check\":\"$1\",\"result\":\"$2\",\"seconds\":$3}")
}

run_check() { # run_check <id> <command...>: bounded, non-interactive, never aborts the run
  local id="$1" started rc=0
  shift
  started="$(date +%s)"
  timeout --kill-after=10 "$check_timeout" "$@" </dev/null >"$work/$id.log" 2>&1 || rc=$?
  local elapsed=$(($(date +%s) - started))
  if [[ $rc -eq 0 ]]; then
    printf 'PASS %-20s %4ss\n' "$id" "$elapsed"
    record "$id" PASSED "$elapsed"
    return 0
  fi
  local state=FAILED
  if [[ $rc -eq 124 || $rc -eq 137 ]]; then state=TIMED_OUT; fi
  printf '%s %-20s %4ss\n' "$state" "$id" "$elapsed"
  tail -n 50 "$work/$id.log" | sed 's/^/    /'
  echo "    (rerun: $rerun)"
  record "$id" "$state" "$elapsed"
  status=1
  return 1
}

if ! tools/bootstrap/preflight.sh "${preflight_args[@]}" >"$work/preflight.log" 2>&1; then
  cat "$work/preflight.log" >&2
  echo "verify: preflight failed; nothing was verified" >&2
  exit 1
fi

if ! "${bazel[@]}" run "${bazel_flags[@]}" --noshow_progress --ui_event_filters=-info,-stdout,-stderr \
  //:tool_paths </dev/null >"$work/tools.env" 2>"$work/tools.log"; then
  tail -n 30 "$work/tools.log" >&2
  echo "verify: could not fetch the pinned tools; nothing was verified (rerun: $rerun)" >&2
  exit 1
fi
set -a
# shellcheck source=/dev/null
. "$work/tools.env"
set +a

tool_version() { # first line of a tool's version output, restricted to safe characters
  "$@" </dev/null 2>&1 | head -1 | tr -cd 'A-Za-z0-9._ +-'
}
bazel_binary="$("${bazel[@]}" --path)"
versions=(
  "\"bazel\":\"$(tool_version "$bazel_binary" --version)\""
  "\"actionlint\":\"$(tool_version "$ACTIONLINT" --version)\""
  "\"buildifier\":\"$(tool_version "$BUILDIFIER" --version)\""
  "\"gitleaks\":\"$(tool_version "$GITLEAKS" --version)\""
  "\"shellcheck\":\"$("$SHELLCHECK" --version </dev/null | sed -n 's/^version: //p' | tr -cd 'A-Za-z0-9._+-')\""
  "\"typos\":\"$(tool_version "$TYPOS" --version)\""
  "\"uv\":\"$(tool_version "$UV" --version)\""
  "\"yq\":\"$("$YQ" --version </dev/null | sed 's/.*version //' | tr -cd 'A-Za-z0-9._+-')\""
  "\"zizmor\":\"$(tool_version "$ZIZMOR" --version)\""
)
if $bootstrap_only; then
  echo "verify: toolchain ready ($(IFS=,; echo "${versions[*]}"))"
  exit 0
fi

expected=(inventory bazel-test workspace-policy implicit-config toolchain-fixtures suppression-ban
  secrets typos shellcheck buildifier actionlint zizmor speckit-provenance)
if ! run_check inventory tools/checks/inventory.sh "$root" "$work"; then
  echo "verify: input discovery failed; no file check can be trusted (rerun: $rerun)" >&2
fi

test_targets=0
if [[ $status -eq 0 ]]; then
  test_targets="$("${bazel[@]}" query --noshow_progress 'tests(//... + @bff_template_quality//...)' 2>/dev/null </dev/null | wc -l | tr -d ' ')" || true
  run_check bazel-test "${bazel[@]}" test "${bazel_flags[@]}" --nocache_test_results --noshow_progress //... @bff_template_quality//... || true
  run_check workspace-policy tools/policy/workspace_policy.sh "$work/all" || true
  run_check implicit-config tools/policy/config_policy.sh "$work/all" || true
  run_check toolchain-fixtures tools/policy/lock_fixture.sh || true
  run_check suppression-ban "$SUPPRESSION" -root "$root" -paths "$work/all" \
    -exempt tools/speckit/scaffold.sha256 -exempt quality/checks/suppression/testdata/fixtures.sha256 || true
  run_check secrets tools/checks/native.sh secrets "$work/all" .gitleaks.toml || true
  run_check typos tools/checks/native.sh typos "$work/authored.nl" _typos.toml || true
  run_check shellcheck tools/checks/native.sh shellcheck "$work/shell" || true
  run_check buildifier tools/checks/native.sh buildifier "$work/starlark" || true
  run_check actionlint tools/checks/native.sh actionlint "$work/workflows" || true
  run_check zizmor tools/checks/native.sh zizmor "$work/workflows" || true
  if $offline; then
    printf 'SKIP %-20s (--offline; not a passing result)\n' speckit-provenance
    record speckit-provenance SKIPPED 0
  else
    run_check speckit-provenance tools/speckit/bootstrap.sh verify || true
  fi
fi
if [[ "$test_targets" -eq 0 ]]; then
  echo "verify: no test targets were found" >&2
  status=1
fi

# The receipt lists every expected check. A check that never ran cannot be mistaken for a pass.
joined="$(IFS=,; echo "${results[*]}")"
for id in "${expected[@]}"; do
  if [[ ",$joined" != *"{\"check\":\"$id\""* ]]; then
    echo "verify: expected check did not run: $id" >&2
    results+=("{\"check\":\"$id\",\"result\":\"NOT_RUN\",\"seconds\":0}")
    status=1
  fi
done
joined="$(IFS=,; echo "${results[*]}")"

revision="$(git rev-parse HEAD)"
tree="$(git rev-parse 'HEAD^{tree}')"
dirty=false
if [[ -n "$(git status --porcelain)" ]]; then dirty=true; fi
digest_of() { xargs -0 sha256sum | LC_ALL=C sort -k2 | sha256sum | cut -d' ' -f1; }
if [[ -s "$work/all" ]]; then
  source_digest="$(digest_of <"$work/all")"
else
  source_digest=none
fi
touch "$work/counts" "$work/all"
grep -zE '^(\.bazelrc|\.bazelversion|MODULE\.bazel(\.lock)?|\.gitleaks\.toml|_typos\.toml|(quality|tools|\.github)/.*)$' \
  "$work/all" >"$work/policy"
policy_digest="$(digest_of <"$work/policy")"
producer=local
if [[ -n "${GITHUB_RUN_ID:-}" ]]; then
  producer="github-actions/${GITHUB_WORKFLOW:-}/${GITHUB_JOB:-}/${GITHUB_RUN_ID}/${GITHUB_RUN_ATTEMPT:-}"
fi
result=FAILED
if [[ $status -eq 0 ]]; then result=PASSED; fi
bazel_sha="$(sha256sum "$bazel_binary" | cut -d' ' -f1)"
counts="$(sed -E 's/^([a-z]+)=([0-9]+)$/"\1":\2,/' "$work/counts" | tr -d '\n')"

mkdir -p .verify
receipt="$work/receipt.json"
cat >"$receipt" <<JSON
{
  "schema_version": 2,
  "revision": "$revision",
  "tree": "$tree",
  "worktree_dirty": $dirty,
  "source_digest": "$source_digest",
  "policy_digest": "$policy_digest",
  "producer": "$producer",
  "isolation": "$isolation",
  "offline": $offline,
  "test_results_cached": false,
  "result": "$result",
  "started_at": "$started_at",
  "finished_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "host": {"os": "$(uname -s)", "arch": "$(uname -m)"},
  "bazel_sha256": "$bazel_sha",
  "tools": {$(IFS=,; echo "${versions[*]}")},
  "inventory": {${counts}"test_targets":$test_targets},
  "expected_checks": ${#expected[@]},
  "checks": [$joined]
}
JSON
if ! "$YQ" -p=json -o=json '.' "$receipt" >/dev/null; then
  echo "verify: the receipt is not valid JSON" >&2
  status=1
fi
cp "$receipt" .verify/receipt.json
state="$result"
if [[ $status -ne 0 ]]; then state=FAILED; fi
note=""
if $dirty; then note=", worktree dirty"; fi
if $offline; then note+=", offline: provenance skipped"; fi
echo "verify: $state at $revision (isolation: $isolation$note)"
exit "$status"
