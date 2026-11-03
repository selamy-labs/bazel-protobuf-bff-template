#!/usr/bin/env bash
# Witness for governance.sh (spec 010 FR-009, FR-009a): the declared target passes offline, then each planted
# defect fails against a copy: a second repository, an import block, a data source, a ruleset active by default,
# a relaxed owner or name pin, and unformatted configuration. Copies dereference links, because runfiles are
# links to the source tree and an edit through one would reach it.
#
#   governance_test.sh <governance.sh> [tofu] [provider zip] [config dir] [ci policy json] [workflow yml]
set -euo pipefail
unset GITHUB_EVENT_NAME GITHUB_EVENT_PATH GITHUB_ACTOR GITHUB_RUN_ID
abs() { case "$1" in /*) printf '%s' "$1" ;; *) printf '%s' "$PWD/$1" ;; esac; }
script="$(abs "${1:?usage: governance_test.sh <governance.sh> [tofu] [provider zip] [config dir] [ci policy json] [workflow yml]}")"

tofu="${2:-}"
if [[ -z "$tofu" ]]; then
  if [[ -n "${RUNFILES_DIR:-}" && -x "${RUNFILES_DIR}/_main/external/opentofu/tofu" ]]; then
    tofu="${RUNFILES_DIR}/_main/external/opentofu/tofu"
  elif [[ -n "${TEST_SRCDIR:-}" && -x "${TEST_SRCDIR}/_main/external/opentofu/tofu" ]]; then
    tofu="${TEST_SRCDIR}/_main/external/opentofu/tofu"
  else
    for c in "${HOME}"/.cache/bazel/*/cache/repos/v1/contents/bd7e472af120153edc82cdaa70300ba812170569a930d56d37878ad394b0b550/*/tofu; do
      if [[ -x "$c" ]]; then tofu="$c"; break; fi
    done
    if [[ -z "$tofu" ]] && command -v tofu >/dev/null 2>&1; then
      if tofu version 2>/dev/null | grep -q "v1\.13\."; then tofu="$(command -v tofu)"; fi
    fi
  fi
fi
[[ -n "$tofu" && -x "$tofu" ]] || { echo "governance_test: cannot locate OpenTofu 1.13.2" >&2; exit 2; }
tofu="$(abs "$tofu")"

zip="${3:-}"
if [[ -z "$zip" ]]; then
  if [[ -n "${RUNFILES_DIR:-}" && -f "${RUNFILES_DIR}/_main/external/+http_file+terraform_provider_github/file/terraform-provider-github_6.13.0_linux_amd64.zip" ]]; then
    zip="${RUNFILES_DIR}/_main/external/+http_file+terraform_provider_github/file/terraform-provider-github_6.13.0_linux_amd64.zip"
  elif [[ -n "${TEST_SRCDIR:-}" && -f "${TEST_SRCDIR}/_main/external/+http_file+terraform_provider_github/file/terraform-provider-github_6.13.0_linux_amd64.zip" ]]; then
    zip="${TEST_SRCDIR}/_main/external/+http_file+terraform_provider_github/file/terraform-provider-github_6.13.0_linux_amd64.zip"
  else
    for z in "${HOME}"/.cache/bazel/*/cache/repos/v1/contents/2eba4ed38ac476c693ec24cefa21936916b9034a6a16632e5d6275a9b103c9f9/*/file/terraform-provider-github_6.13.0_linux_amd64.zip; do
      if [[ -f "$z" ]]; then zip="$z"; break; fi
    done
  fi
fi
[[ -n "$zip" && -f "$zip" ]] || { echo "governance_test: cannot locate terraform-provider-github_6.13.0_linux_amd64.zip" >&2; exit 2; }
zip="$(abs "$zip")"

config="${4:-}"
if [[ -z "$config" ]]; then
  if [[ -d "governance/github" ]]; then
    config="governance/github"
  elif [[ -d "${0%/*}/../../governance/github" ]]; then
    config="${0%/*}/../../governance/github"
  fi
fi
[[ -n "$config" && -d "$config" ]] || { echo "governance_test: cannot locate governance/github directory" >&2; exit 2; }
config="$(abs "$config")"

policy="${5:-}"
if [[ -z "$policy" ]]; then
  if [[ -f "tools/ci/policy.json" ]]; then
    policy="tools/ci/policy.json"
  elif [[ -f "${0%/*}/../ci/policy.json" ]]; then
    policy="${0%/*}/../ci/policy.json"
  fi
fi
[[ -n "$policy" && -f "$policy" ]] || { echo "governance_test: cannot locate tools/ci/policy.json" >&2; exit 2; }
policy="$(abs "$policy")"

workflow="${6:-}"
if [[ -z "$workflow" ]]; then
  if [[ -f ".github/workflows/ci.yml" ]]; then
    workflow=".github/workflows/ci.yml"
  elif [[ -f "${0%/*}/../../.github/workflows/ci.yml" ]]; then
    workflow="${0%/*}/../../.github/workflows/ci.yml"
  fi
fi
[[ -n "$workflow" && -f "$workflow" ]] || { echo "governance_test: cannot locate .github/workflows/ci.yml" >&2; exit 2; }
workflow="$(abs "$workflow")"

T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
failures=0
replace() { local content; content="$(cat "$1")"; printf '%s\n' "${content/"$2"/"$3"}" >"$1"; }
expect() { # expect <name> <want> <mutation> [tofu]
  local name="$1" want="$2" rc=0
  rm -rf "$T/g"; cp -rL "$config" "$T/g"; cp -L "$policy" "$T/policy.json"; cp -L "$workflow" "$T/ci.yml"
  case "$3" in
    none) ;;
    second_repository) printf '\nresource "github_repository" "second" {\n  name = "other"\n}\n' >>"$T/g/main.tf" ;;
    import_block) printf '\nimport {\n  to = github_repository.template\n  id = "existing"\n}\n' >>"$T/g/main.tf" ;;
    data_source) printf '\ndata "github_repository" "existing" {\n  name = "x"\n}\n' >>"$T/g/main.tf" ;;
    active_by_default) replace "$T/g/variables.tf" 'default     = false' 'default     = true' ;;
    relax_owner) replace "$T/g/variables.tf" 'var.owner == "selamy-labs"' 'var.owner != ""' ;;
    relax_name) replace "$T/g/variables.tf" 'var.name == "bazel-protobuf-bff-template"' 'var.name != ""' ;;
    unformatted) replace "$T/g/main.tf" '  name        = var.name' '  name=var.name' ;;
    json_resource) printf '{"resource":{"github_repository":{"third":{"name":"third"}}}}\n' >"$T/g/extra.tf.json" ;;
    provider_literal) replace "$T/g/main.tf" '  owner = var.owner' '  owner = "someone-else"' ;;
    ruleset_other) replace "$T/g/main.tf" '  repository  = github_repository.template.name' '  repository  = "other-existing-repository"' ;;
    context_other) replace "$T/g/main.tf" '        context = "gate"' '        context = "aggregate"' ;;
    context_blank) replace "$T/g/main.tf" '        context = "gate"' '        context = ""' ;;
    policy_gate_other) replace "$T/policy.json" '"gate": "gate",' '"gate": "aggregate",' ;;
    workflow_gate_renamed) replace "$T/ci.yml" '    name: gate' '    name: aggregate' ;;
    squash_merge_allowed) replace "$T/g/main.tf" '  allow_squash_merge     = false' '  allow_squash_merge     = true' ;;
    rebase_merge_disallowed) replace "$T/g/main.tf" '  allow_rebase_merge     = true' '  allow_rebase_merge     = false' ;;
    ruleset_squash_allowed) replace "$T/g/main.tf" 'allowed_merge_methods           = ["rebase"]' 'allowed_merge_methods           = ["rebase", "squash"]' ;;
    ruleset_no_committer_email_pattern) sed -i '/committer_email_pattern {/,/}/d' "$T/g/main.tf" ;;
    ruleset_committer_pattern_all) replace "$T/g/main.tf" 'pattern  = "^(noreply@github\\.com|346154065\\+selamy-labs-bot@users\\.noreply\\.github\\.com|claude-cir@selamy\\.dev)$"' 'pattern  = ".*"' ;;
    ruleset_committer_pattern_broad) replace "$T/g/main.tf" 'pattern  = "^(noreply@github\\.com|346154065\\+selamy-labs-bot@users\\.noreply\\.github\\.com|claude-cir@selamy\\.dev)$"' 'pattern  = ".*@users\\.noreply\\.github\\.com"' ;;
    ruleset_committer_negate_true) replace "$T/g/main.tf" 'negate   = false' 'negate   = true' ;;
    ruleset_committer_comment_shadow) sed -i '/committer_email_pattern/,/}/s,pattern  = .*,pattern  = ".*"\n      # &,g' "$T/g/main.tf"; "$tofu" -chdir="$T/g" fmt >/dev/null 2>&1 ;;
    ruleset_committer_slash_comment_shadow) sed -i '/committer_email_pattern/,/}/s,pattern  = .*,pattern  = ".*"\n      // &,g' "$T/g/main.tf"; "$tofu" -chdir="$T/g" fmt >/dev/null 2>&1 ;;
    ruleset_committer_block_comment_shadow) sed -i '/committer_email_pattern/,/}/s,pattern  = .*,pattern  = ".*" /* & */,g' "$T/g/main.tf"; "$tofu" -chdir="$T/g" fmt >/dev/null 2>&1 ;;
    ruleset_no_commit_author_email_pattern) sed -i '/commit_author_email_pattern {/,/}/d' "$T/g/main.tf" ;;
    ruleset_author_pattern_bot) replace "$T/g/main.tf" 'pattern  = "^claude-cir@selamy\\.dev$"' 'pattern  = "^(346154065\\+selamy-labs-bot@users\\.noreply\\.github\\.com|claude-cir@selamy\\.dev)$"' ;;
    ruleset_author_pattern_all) replace "$T/g/main.tf" 'pattern  = "^claude-cir@selamy\\.dev$"' 'pattern  = ".*"' ;;
    ruleset_author_negate_true) sed -i '/commit_author_email_pattern/,/}/s/negate   = false/negate   = true/' "$T/g/main.tf" ;;
    ruleset_author_comment_shadow) sed -i '/commit_author_email_pattern/,/}/s,pattern  = .*,pattern  = ".*"\n      # &,g' "$T/g/main.tf"; "$tofu" -chdir="$T/g" fmt >/dev/null 2>&1 ;;
    ruleset_author_slash_comment_shadow) sed -i '/commit_author_email_pattern/,/}/s,pattern  = .*,pattern  = ".*"\n      // &,g' "$T/g/main.tf"; "$tofu" -chdir="$T/g" fmt >/dev/null 2>&1 ;;
    ruleset_author_block_comment_shadow) sed -i '/commit_author_email_pattern/,/}/s,pattern  = .*,pattern  = ".*" /* & */,g' "$T/g/main.tf"; "$tofu" -chdir="$T/g" fmt >/dev/null 2>&1 ;;
    module_block) printf '\nmodule "extra" {\n  source = "./extra"\n}\n' >>"$T/g/main.tf"; mkdir -p "$T/g/extra"; printf 'resource "github_repository" "third" {\n  name = "third"\n}\n' >"$T/g/extra/main.tf" ;;
    *) echo "unknown mutation $3"; exit 2 ;;
  esac
  "$script" "${4:-$tofu}" "$zip" "$T/g" "$T/policy.json" "$T/ci.yml" >"$T/out" 2>&1 || rc=$?
  if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; tail -5 "$T/out" | sed 's/^/     /'; failures=$((failures + 1)); fi
}
expect "the declared target passes offline" 0 none
grep -q 'offline' "$T/out" || { echo "FAIL the pass is not stated"; failures=$((failures + 1)); }
expect "a second repository" 1 second_repository
expect "an import block" 1 import_block
expect "a data source" 1 data_source
expect "a ruleset active by default" 1 active_by_default
expect "the pinned owner relaxed" 1 relax_owner
expect "the pinned name relaxed" 1 relax_name
expect "unformatted configuration" 1 unformatted
expect "a third resource in JSON configuration" 1 json_resource
expect "a module that adds a resource" 1 module_block
expect "the provider bound to a literal owner" 1 provider_literal
expect "the ruleset bound to another repository" 1 ruleset_other
# The required check must be the gate job: the defect an independent read-only audit found before publication
expect "a required check that names no job" 1 context_other
expect "a blank required check" 1 context_blank
expect "the policy's gate renamed under the ruleset" 1 policy_gate_other
expect "the workflow's gate job renamed under the ruleset" 1 workflow_gate_renamed
expect "squash merge allowed" 1 squash_merge_allowed
expect "rebase merge disallowed" 1 rebase_merge_disallowed
expect "ruleset allows squash in allowed_merge_methods" 1 ruleset_squash_allowed
expect "ruleset removes committer_email_pattern" 1 ruleset_no_committer_email_pattern
expect "ruleset with committer pattern .* is rejected" 1 ruleset_committer_pattern_all
expect "ruleset with committer pattern .*@users.noreply.github.com is rejected" 1 ruleset_committer_pattern_broad
expect "ruleset with committer negate = true is rejected" 1 ruleset_committer_negate_true
expect "ruleset with committer trailing comment shadow is rejected" 1 ruleset_committer_comment_shadow
grep -q 'ruleset must declare exact committer_email_pattern and commit_author_email_pattern' "$T/out" || { echo "FAIL ruleset_committer_comment_shadow: expected pattern error, got: $(cat "$T/out")"; failures=$((failures + 1)); }
expect "ruleset with committer trailing slash comment shadow is rejected" 1 ruleset_committer_slash_comment_shadow
grep -q 'ruleset must declare exact committer_email_pattern and commit_author_email_pattern' "$T/out" || { echo "FAIL ruleset_committer_slash_comment_shadow: expected pattern error, got: $(cat "$T/out")"; failures=$((failures + 1)); }
expect "ruleset with committer trailing block comment shadow is rejected" 1 ruleset_committer_block_comment_shadow
grep -q 'ruleset must declare exact committer_email_pattern and commit_author_email_pattern' "$T/out" || { echo "FAIL ruleset_committer_block_comment_shadow: expected pattern error, got: $(cat "$T/out")"; failures=$((failures + 1)); }
expect "ruleset removes commit_author_email_pattern" 1 ruleset_no_commit_author_email_pattern
expect "ruleset with author pattern accepting bot is rejected" 1 ruleset_author_pattern_bot
expect "ruleset with author pattern .* is rejected" 1 ruleset_author_pattern_all
expect "ruleset with author negate = true is rejected" 1 ruleset_author_negate_true
expect "ruleset with author trailing comment shadow is rejected" 1 ruleset_author_comment_shadow
grep -q 'ruleset must declare exact committer_email_pattern and commit_author_email_pattern' "$T/out" || { echo "FAIL ruleset_author_comment_shadow: expected pattern error, got: $(cat "$T/out")"; failures=$((failures + 1)); }
expect "ruleset with author trailing slash comment shadow is rejected" 1 ruleset_author_slash_comment_shadow
grep -q 'ruleset must declare exact committer_email_pattern and commit_author_email_pattern' "$T/out" || { echo "FAIL ruleset_author_slash_comment_shadow: expected pattern error, got: $(cat "$T/out")"; failures=$((failures + 1)); }
expect "ruleset with author trailing block comment shadow is rejected" 1 ruleset_author_block_comment_shadow
grep -q 'ruleset must declare exact committer_email_pattern and commit_author_email_pattern' "$T/out" || { echo "FAIL ruleset_author_block_comment_shadow: expected pattern error, got: $(cat "$T/out")"; failures=$((failures + 1)); }
expect "no tofu" 11 none "$T/absent"
expect "restored control" 0 none

# PR-time actor ratchet tests
printf '{"pull_request": {"user": {"login": "pselamy"}}}\n' >"$T/event-bad.json"
printf '{"pull_request": {"user": {"login": "selamy-labs-bot"}}}\n' >"$T/event-bot.json"
printf '{"pull_request": {"user": {"login": "claude-cir"}}}\n' >"$T/event-claude.json"
printf '{"pull_request": {"user": {"login": "dependabot[bot]"}}}\n' >"$T/event-dependabot.json"
printf '{"pull_request": {"user": {"login": "evil-app[bot]"}}}\n' >"$T/event-evil-bot.json"
printf '{"merge_group": {"actor": {"login": "pselamy"}}}\n' >"$T/event-mg-human.json"
printf '{"merge_group": {"actor": {"login": "selamy-labs-bot"}}}\n' >"$T/event-mg-bot.json"
printf '{bad\n' >"$T/event-malformed.json"
printf '{"pull_request": {"user": {}}}\n' >"$T/event-missing-user.json"

expect_event() { # expect_event <name> <want> <event_file> [extra_args...]
  local name="$1" want="$2" event_file="$3" rc=0; shift 3
  "$script" "$tofu" "$zip" "$config" "$policy" "$workflow" --event pull_request --event-path "$event_file" "$@" >"$T/out" 2>&1 || rc=$?
  if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; tail -5 "$T/out" | sed 's/^/     /'; failures=$((failures + 1)); fi
}

expect_event_env() { # expect_event_env <name> <want> <event_file> [env_vars...]
  local name="$1" want="$2" event_file="$3" rc=0; shift 3
  env GITHUB_EVENT_NAME=pull_request GITHUB_EVENT_PATH="$event_file" "$@" "$script" "$tofu" "$zip" "$config" "$policy" "$workflow" >"$T/out" 2>&1 || rc=$?
  if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; tail -5 "$T/out" | sed 's/^/     /'; failures=$((failures + 1)); fi
}

expect_env_only() { # expect_env_only <name> <want> [env_vars...]
  local name="$1" want="$2" rc=0; shift 2
  env GITHUB_EVENT_NAME=pull_request "$@" "$script" "$tofu" "$zip" "$config" "$policy" "$workflow" >"$T/out" 2>&1 || rc=$?
  if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; tail -5 "$T/out" | sed 's/^/     /'; failures=$((failures + 1)); fi
}

expect_event "PR event payload with personal user login fails governance.sh" 1 "$T/event-bad.json"
expect_event_env "PR event payload with personal user login fails governance.sh via env" 1 "$T/event-bad.json"

# Positive controls for all 3 approved identities
expect_event "PR event payload with selamy-labs-bot login passes governance.sh" 0 "$T/event-bot.json"
expect_event_env "PR event payload with selamy-labs-bot login passes governance.sh via env" 0 "$T/event-bot.json" GITHUB_ACTOR=selamy-labs-bot
expect_event "PR event payload with claude-cir login passes governance.sh" 0 "$T/event-claude.json"
expect_event_env "PR event payload with claude-cir login passes governance.sh via env" 0 "$T/event-claude.json" GITHUB_ACTOR=claude-cir
expect_event "PR event payload with dependabot[bot] login passes governance.sh" 0 "$T/event-dependabot.json"
expect_event_env "PR event payload with dependabot[bot] login passes governance.sh via env" 0 "$T/event-dependabot.json" GITHUB_ACTOR="dependabot[bot]"

# Negative controls for actor ratchet and payload validation
expect_event_env "GITHUB_ACTOR with personal user fails governance.sh via env" 1 "$T/event-bot.json" GITHUB_ACTOR=pselamy
expect_env_only "GITHUB_ACTOR=pselamy without GITHUB_EVENT_PATH fails governance.sh" 1 GITHUB_ACTOR=pselamy
expect_env_only "unreadable GITHUB_EVENT_PATH fails governance.sh" 1 GITHUB_EVENT_PATH=/nonexistent/path GITHUB_ACTOR=selamy-labs-bot
expect_event_env "malformed JSON in GITHUB_EVENT_PATH fails governance.sh" 1 "$T/event-malformed.json" GITHUB_ACTOR=selamy-labs-bot
expect_event_env "missing pull_request user login in payload fails governance.sh" 1 "$T/event-missing-user.json" GITHUB_ACTOR=selamy-labs-bot
expect_event_env "missing GITHUB_ACTOR in CI (GITHUB_RUN_ID set) fails governance.sh" 1 "$T/event-bot.json" GITHUB_RUN_ID=12345
expect_env_only "missing GITHUB_EVENT_PATH in CI (GITHUB_RUN_ID set) fails governance.sh" 1 GITHUB_RUN_ID=12345 GITHUB_ACTOR=selamy-labs-bot
expect_env_only "GITHUB_ACTOR with unapproved bot evil-app[bot] fails governance.sh" 1 GITHUB_ACTOR="evil-app[bot]"
expect_event_env "PR event payload with evil-app[bot] login fails governance.sh via env" 1 "$T/event-evil-bot.json" GITHUB_ACTOR=selamy-labs-bot
expect_env_only "merge_group with human actor pselamy fails governance.sh" 1 GITHUB_EVENT_NAME=merge_group GITHUB_ACTOR=pselamy
expect_event_env "merge_group event payload with human actor pselamy fails governance.sh" 1 "$T/event-mg-human.json" GITHUB_EVENT_NAME=merge_group GITHUB_ACTOR=selamy-labs-bot
expect_event_env "merge_group with approved bot actor passes governance.sh" 0 "$T/event-mg-bot.json" GITHUB_EVENT_NAME=merge_group GITHUB_ACTOR=selamy-labs-bot

# Unit tests for merge_pr.sh resolved from sibling path of $1 (governance.sh)
merge_pr_script="$(dirname "$script")/merge_pr.sh"
[[ -x "$merge_pr_script" ]] || {
  echo "governance_test: canonical merge helper merge_pr.sh is missing or not executable at sibling path $merge_pr_script" >&2
  exit 2
}

mkdir -p "$T/bin"
gh_login_file="$T/gh-caller-login"
gh_fail_file="$T/gh-fail-flag"
gh_merge_log="$T/gh-merge.log"

cat >"$T/bin/gh" <<GH_STUB
#!/usr/bin/env bash
set -euo pipefail
if [[ "\$*" == "api user --jq .login" ]]; then
  if [[ -f "$gh_fail_file" ]]; then
    exit 1
  fi
  cat "$gh_login_file"
  exit 0
fi
if [[ "\$1" == "pr" && "\$2" == "merge" ]]; then
  echo "\$*" >>"$gh_merge_log"
  exit 0
fi
echo "unexpected gh call: \$*" >&2
exit 2
GH_STUB
chmod +x "$T/bin/gh"

expect_merge() { # expect_merge <name> <want> <login> [args...]
  local name="$1" want="$2" login="$3" rc=0; shift 3
  rm -f "$gh_fail_file" "$gh_merge_log"
  if [[ "$login" == "__FAIL__" ]]; then
    touch "$gh_fail_file"
    : >"$gh_login_file"
  else
    printf '%s\n' "$login" >"$gh_login_file"
  fi
  env PATH="$T/bin:$PATH" "$merge_pr_script" "$@" >"$T/out" 2>&1 || rc=$?
  if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; tail -5 "$T/out" | sed 's/^/     /'; failures=$((failures + 1)); fi
}

expect_merge "merge_pr.sh rejects personal user pselamy" 1 "pselamy" 42 --match-head-commit abc1234
grep -qF "caller 'pselamy' is not permitted" "$T/out" || { echo "FAIL expected caller 'pselamy' in rejection message"; failures=$((failures + 1)); }
expect_merge "merge_pr.sh rejects empty caller login" 1 "" 42 --match-head-commit abc1234
expect_merge "merge_pr.sh rejects failed gh api user command" 1 "__FAIL__" 42 --match-head-commit abc1234
expect_merge "merge_pr.sh succeeds for selamy-labs-bot" 0 "selamy-labs-bot" 42 --match-head-commit abc1234
grep -qF "pr merge 42 --rebase --match-head-commit abc1234" "$gh_merge_log" || { echo "FAIL expected merge command in log"; failures=$((failures + 1)); }
expect_merge "merge_pr.sh succeeds for claude-cir with --repo" 0 "claude-cir" 42 --match-head-commit abc1234 --repo selamy-labs/bazel-protobuf-bff-template
grep -qF "pr merge 42 --rebase --match-head-commit abc1234 --repo selamy-labs/bazel-protobuf-bff-template" "$gh_merge_log" || { echo "FAIL expected merge command with repo in log"; failures=$((failures + 1)); }

# Negative test: governance.sh --merge-pr fails closed without merge_pr.sh, with zero gh calls
missing_helper_dir="$T/missing-helper"
mkdir -p "$missing_helper_dir"
cp "$script" "$missing_helper_dir/governance.sh"
rm -f "$gh_merge_log"
missing_rc=0
env PATH="$T/bin:$PATH" bash "$missing_helper_dir/governance.sh" --merge-pr 42 --squash --admin >"$T/out" 2>&1 || missing_rc=$?
if [[ $missing_rc -ne 0 && ! -s "$gh_merge_log" ]]; then
  echo "ok   governance.sh --merge-pr fails closed without merge_pr.sh with zero gh merge calls"
else
  echo "FAIL governance.sh --merge-pr did not fail closed without merge_pr.sh: exit $missing_rc"
  failures=$((failures + 1))
fi

exit "$failures"
