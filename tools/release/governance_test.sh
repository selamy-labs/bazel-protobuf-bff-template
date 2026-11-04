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
    ruleset_committer_pattern_all) replace "$T/g/main.tf" 'pattern  = "^(noreply@github\\.com|github-bot@selamy\\.dev|claude-cir@selamy\\.dev)$"' 'pattern  = ".*"' ;;
    ruleset_committer_pattern_broad) replace "$T/g/main.tf" 'pattern  = "^(noreply@github\\.com|github-bot@selamy\\.dev|claude-cir@selamy\\.dev)$"' 'pattern  = ".*@selamy\\.dev"' ;;
    ruleset_committer_negate_true) replace "$T/g/main.tf" 'negate   = false' 'negate   = true' ;;
    ruleset_committer_comment_shadow) sed -i '/committer_email_pattern/,/}/s,pattern  = .*,pattern  = ".*"\n      # &,g' "$T/g/main.tf"; "$tofu" -chdir="$T/g" fmt >/dev/null 2>&1 ;;
    ruleset_committer_slash_comment_shadow) sed -i '/committer_email_pattern/,/}/s,pattern  = .*,pattern  = ".*"\n      // &,g' "$T/g/main.tf"; "$tofu" -chdir="$T/g" fmt >/dev/null 2>&1 ;;
    ruleset_committer_block_comment_shadow) sed -i '/committer_email_pattern/,/}/s,pattern  = .*,pattern  = ".*" /* & */,g' "$T/g/main.tf"; "$tofu" -chdir="$T/g" fmt >/dev/null 2>&1 ;;
    ruleset_no_commit_author_email_pattern) sed -i '/commit_author_email_pattern {/,/}/d' "$T/g/main.tf" ;;
    ruleset_author_pattern_bot) replace "$T/g/main.tf" 'pattern  = "^claude-cir@selamy\\.dev$"' 'pattern  = "^(github-bot@selamy\\.dev|claude-cir@selamy\\.dev)$"' ;;
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
  orig_args=("\$@")
  shift 2
  while [[ \$# -gt 0 ]]; do
    if [[ "\$1" == "--repo" ]]; then
      shift
      repo_val="\${1:-}"
      if [[ "\$repo_val" == /* || "\$repo_val" == ./* || "\$repo_val" == ../* || "\$repo_val" == "." ]]; then
        echo "gh: invalid repository selector '\$repo_val': expected [HOST/]OWNER/REPO" >&2
        exit 1
      fi
    elif [[ "\$1" == --repo=* ]]; then
      repo_val="\${1#--repo=}"
      if [[ "\$repo_val" == /* || "\$repo_val" == ./* || "\$repo_val" == ../* || "\$repo_val" == "." ]]; then
        echo "gh: invalid repository selector '\$repo_val': expected [HOST/]OWNER/REPO" >&2
        exit 1
      fi
    fi
    shift
  done
  echo "\${orig_args[*]}" >>"$gh_merge_log"
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

expect_merge_in_repo() { # expect_merge_in_repo <dir> <name> <want> <login> [args...]
  local dir="$1" name="$2" want="$3" login="$4" rc=0; shift 4
  rm -f "$gh_fail_file" "$gh_merge_log"
  if [[ "$login" == "__FAIL__" ]]; then
    touch "$gh_fail_file"
    : >"$gh_login_file"
  else
    printf '%s\n' "$login" >"$gh_login_file"
  fi
  (cd "$dir" && env PATH="$T/bin:$PATH" "$merge_pr_script" "$@" >"$T/out" 2>&1) || rc=$?
  if [[ "$rc" == "$want" ]]; then echo "ok   $name"; else echo "FAIL $name: exit $rc"; tail -5 "$T/out" | sed 's/^/     /'; failures=$((failures + 1)); fi
}

# Set up clean fixture repo for merge_pr.sh tests
merge_repo="$T/merge-repo"
git init -q -b main "$merge_repo"
(cd "$merge_repo" && git remote add origin https://github.com/selamy-labs/bazel-protobuf-bff-template.git)
(cd "$merge_repo" && printf 'initial\n' >a.txt && git add . && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="github-bot@selamy.dev" git commit -q -m "initial bot merge commit")

mkdir -p "$merge_repo/tools/release"
allow_src="tools/release/neutrality.allow"
[[ -f "$allow_src" ]] || allow_src="${script%/*}/neutrality.allow"
if [[ ! -f "$allow_src" && -n "${RUNFILES_DIR:-}" ]]; then
  allow_src="${RUNFILES_DIR}/_main/tools/release/neutrality.allow"
fi
cp "$allow_src" "$merge_repo/tools/release/neutrality.allow"

neut_src="tools/release/neutrality.sh"
[[ -f "$neut_src" ]] || neut_src="${script%/*}/neutrality.sh"
if [[ ! -f "$neut_src" && -n "${RUNFILES_DIR:-}" ]]; then
  neut_src="${RUNFILES_DIR}/_main/tools/release/neutrality.sh"
fi
cp "$neut_src" "$merge_repo/tools/release/neutrality.sh"

vrm_src="tools/release/verify_rebase_merge.sh"
[[ -f "$vrm_src" ]] || vrm_src="${script%/*}/verify_rebase_merge.sh"
if [[ ! -f "$vrm_src" && -n "${RUNFILES_DIR:-}" ]]; then
  vrm_src="${RUNFILES_DIR}/_main/tools/release/verify_rebase_merge.sh"
fi
cp "$vrm_src" "$merge_repo/tools/release/verify_rebase_merge.sh"
chmod +x "$merge_repo/tools/release/"*.sh

(cd "$merge_repo" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q --allow-empty -m "clean candidate commit")
clean_candidate_sha="$(git -C "$merge_repo" rev-parse HEAD)"

# Create a separate isolated repo for testing a candidate that fails neutrality preflight
bad_merge_repo="$T/bad-merge-repo"
git clone -q "$merge_repo" "$bad_merge_repo"
(cd "$bad_merge_repo" && GIT_AUTHOR_NAME="Patrick Selamy" GIT_AUTHOR_EMAIL="pselamy@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q --allow-empty -m "bad candidate commit")
bad_candidate_sha="$(git -C "$bad_merge_repo" rev-parse HEAD)"

# Negative caller authentication tests
expect_merge_in_repo "$merge_repo" "merge_pr.sh rejects personal user pselamy" 1 "pselamy" 42 --match-head-commit "$clean_candidate_sha"
grep -qF "caller 'pselamy' is not permitted" "$T/out" || { echo "FAIL expected caller 'pselamy' in rejection message"; failures=$((failures + 1)); }
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for personal caller"; failures=$((failures + 1)); }

expect_merge_in_repo "$merge_repo" "merge_pr.sh rejects empty caller login" 1 "" 42 --match-head-commit "$clean_candidate_sha"
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for empty caller"; failures=$((failures + 1)); }

expect_merge_in_repo "$merge_repo" "merge_pr.sh rejects failed gh api user command" 1 "__FAIL__" 42 --match-head-commit "$clean_candidate_sha"
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for failed caller"; failures=$((failures + 1)); }

# Negative preflight validation tests for all invocation forms (SOL-R13-02, SOL-R13-04)
expect_merge_in_repo "$merge_repo" "merge_pr.sh fails when candidate head cannot be resolved locally" 1 "selamy-labs-bot" 42 --match-head-commit nonexistent1234
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for unresolvable commit"; failures=$((failures + 1)); }

# Form 1: owner/repo
expect_merge_in_repo "$bad_merge_repo" "merge_pr.sh fails when candidate head fails neutrality preflight (owner/repo form)" 1 "selamy-labs-bot" 42 --match-head-commit "$bad_candidate_sha" --repo selamy-labs/bazel-protobuf-bff-template
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for failing preflight (owner/repo)"; failures=$((failures + 1)); }

# Form 2: URL
expect_merge_in_repo "$bad_merge_repo" "merge_pr.sh fails when candidate head fails neutrality preflight (URL form)" 1 "selamy-labs-bot" 42 --match-head-commit "$bad_candidate_sha" --repo https://github.com/selamy-labs/bazel-protobuf-bff-template.git
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for failing preflight (URL)"; failures=$((failures + 1)); }

# Form 3: local path
expect_merge "merge_pr.sh fails when candidate head fails neutrality preflight (local path form)" 1 "selamy-labs-bot" 42 --match-head-commit "$bad_candidate_sha" --repo "$bad_merge_repo"
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for failing preflight (local path)"; failures=$((failures + 1)); }

# Form 4: default local checkout without --repo
expect_merge_in_repo "$bad_merge_repo" "merge_pr.sh fails when candidate head fails neutrality preflight (default form)" 1 "selamy-labs-bot" 42 --match-head-commit "$bad_candidate_sha"
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for failing preflight (default)"; failures=$((failures + 1)); }
rm -rf "$bad_merge_repo"

# Negative test: local repository without a remote cannot resolve GitHub repository selector
merge_repo_no_remote="$T/merge-repo-no-remote"
git init -q -b main "$merge_repo_no_remote"
(cd "$merge_repo_no_remote" && printf 'initial\n' >a.txt && git add . && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="github-bot@selamy.dev" git commit -q -m "initial bot merge commit")
mkdir -p "$merge_repo_no_remote/tools/release"
cp "$allow_src" "$merge_repo_no_remote/tools/release/neutrality.allow"
cp "$neut_src" "$merge_repo_no_remote/tools/release/neutrality.sh"
cp "$vrm_src" "$merge_repo_no_remote/tools/release/verify_rebase_merge.sh"
chmod +x "$merge_repo_no_remote/tools/release/"*.sh
(cd "$merge_repo_no_remote" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q --allow-empty -m "clean candidate commit")
clean_no_remote_sha="$(git -C "$merge_repo_no_remote" rev-parse HEAD)"

expect_merge "merge_pr.sh fails for local repository without remote" 1 "dependabot[bot]" 42 --match-head-commit "$clean_no_remote_sha" --repo "$merge_repo_no_remote"
grep -qF "cannot resolve GitHub repository selector" "$T/out" || { echo "FAIL expected cannot resolve remote in output"; failures=$((failures + 1)); }
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for repo without remote"; failures=$((failures + 1)); }
rm -rf "$merge_repo_no_remote"

# Negative test: local repository with filesystem remote is refused (BFF-RM14-02)
merge_repo_fs_remote="$T/merge-repo-fs-remote"
git init -q -b main "$merge_repo_fs_remote"
(cd "$merge_repo_fs_remote" && git remote add origin "/tmp/fake-filesystem-remote")
(cd "$merge_repo_fs_remote" && printf 'initial\n' >a.txt && git add . && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="github-bot@selamy.dev" git commit -q -m "initial bot merge commit")
mkdir -p "$merge_repo_fs_remote/tools/release"
cp "$allow_src" "$merge_repo_fs_remote/tools/release/neutrality.allow"
cp "$neut_src" "$merge_repo_fs_remote/tools/release/neutrality.sh"
cp "$vrm_src" "$merge_repo_fs_remote/tools/release/verify_rebase_merge.sh"
chmod +x "$merge_repo_fs_remote/tools/release/"*.sh
(cd "$merge_repo_fs_remote" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q --allow-empty -m "clean candidate commit")
clean_fs_sha="$(git -C "$merge_repo_fs_remote" rev-parse HEAD)"

expect_merge "merge_pr.sh fails for local repository with filesystem remote" 1 "dependabot[bot]" 42 --match-head-commit "$clean_fs_sha" --repo "$merge_repo_fs_remote"
grep -qF "cannot resolve GitHub repository selector" "$T/out" || { echo "FAIL expected cannot resolve remote in output for filesystem remote"; failures=$((failures + 1)); }
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for filesystem remote"; failures=$((failures + 1)); }
rm -rf "$merge_repo_fs_remote"

# Negative test: local repository with non-GitHub GitLab remote is refused (BFF-RM14-02)
merge_repo_gitlab="$T/merge-repo-gitlab"
git init -q -b main "$merge_repo_gitlab"
(cd "$merge_repo_gitlab" && git remote add origin "https://gitlab.example.com/selamy-labs/bazel-protobuf-bff-template.git")
(cd "$merge_repo_gitlab" && printf 'initial\n' >a.txt && git add . && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="github-bot@selamy.dev" git commit -q -m "initial bot merge commit")
mkdir -p "$merge_repo_gitlab/tools/release"
cp "$allow_src" "$merge_repo_gitlab/tools/release/neutrality.allow"
cp "$neut_src" "$merge_repo_gitlab/tools/release/neutrality.sh"
cp "$vrm_src" "$merge_repo_gitlab/tools/release/verify_rebase_merge.sh"
chmod +x "$merge_repo_gitlab/tools/release/"*.sh
(cd "$merge_repo_gitlab" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q --allow-empty -m "clean candidate commit")
clean_gitlab_sha="$(git -C "$merge_repo_gitlab" rev-parse HEAD)"

expect_merge "merge_pr.sh fails for local repository with GitLab remote" 1 "dependabot[bot]" 42 --match-head-commit "$clean_gitlab_sha" --repo "$merge_repo_gitlab"
grep -qF "cannot resolve GitHub repository selector" "$T/out" || { echo "FAIL expected cannot resolve remote in output for gitlab remote"; failures=$((failures + 1)); }
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for gitlab remote"; failures=$((failures + 1)); }
rm -rf "$merge_repo_gitlab"

# Positive invocation form tests proving preflight ran (SOL-R13-02)
# Form 1: owner/repo
expect_merge_in_repo "$merge_repo" "merge_pr.sh succeeds for selamy-labs-bot with owner/repo form" 0 "selamy-labs-bot" 42 --match-head-commit "$clean_candidate_sha" --repo selamy-labs/bazel-protobuf-bff-template
grep -qF "pr merge 42 --rebase --match-head-commit $clean_candidate_sha --repo selamy-labs/bazel-protobuf-bff-template" "$gh_merge_log" || { echo "FAIL expected merge command with owner/repo in log"; failures=$((failures + 1)); }

# Form 2: URL
expect_merge_in_repo "$merge_repo" "merge_pr.sh succeeds for claude-cir with URL form" 0 "claude-cir" 42 --match-head-commit "$clean_candidate_sha" --repo https://github.com/selamy-labs/bazel-protobuf-bff-template.git
grep -qF "pr merge 42 --rebase --match-head-commit $clean_candidate_sha --repo selamy-labs/bazel-protobuf-bff-template" "$gh_merge_log" || { echo "FAIL expected merge command with URL in log"; failures=$((failures + 1)); }

# Form 3: local path resolves GitHub remote and normalizes --repo selector
expect_merge "merge_pr.sh succeeds for dependabot[bot] with local path form" 0 "dependabot[bot]" 42 --match-head-commit "$clean_candidate_sha" --repo "$merge_repo"
grep -qF "pr merge 42 --rebase --match-head-commit $clean_candidate_sha --repo selamy-labs/bazel-protobuf-bff-template" "$gh_merge_log" || { echo "FAIL expected merge command with normalized repo in log"; failures=$((failures + 1)); }

# Form 4: default local checkout without --repo
expect_merge_in_repo "$merge_repo" "merge_pr.sh succeeds for selamy-labs-bot without --repo" 0 "selamy-labs-bot" 42 --match-head-commit "$clean_candidate_sha"
grep -qF "pr merge 42 --rebase --match-head-commit $clean_candidate_sha --repo selamy-labs/bazel-protobuf-bff-template" "$gh_merge_log" || { echo "FAIL expected merge command in log"; failures=$((failures + 1)); }

# Form 5: remote named upstream fetches missing candidate head and merges successfully (SOL-R14-01)
upstream_source="$T/upstream-source-repo"
git clone -q "$merge_repo" "$upstream_source"
(cd "$upstream_source" && printf 'upstream-change\n' >u.txt && git add u.txt && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q -m "upstream candidate commit")
upstream_candidate_sha="$(git -C "$upstream_source" rev-parse HEAD)"

upstream_only_repo="$T/upstream-only-repo"
git init -q -b main "$upstream_only_repo"
git -C "$upstream_only_repo" fetch -q "$merge_repo" main
git -C "$upstream_only_repo" reset -q --hard FETCH_HEAD
mkdir -p "$upstream_only_repo/tools/release"
cp "$allow_src" "$upstream_only_repo/tools/release/neutrality.allow"
cp "$neut_src" "$upstream_only_repo/tools/release/neutrality.sh"
cp "$vrm_src" "$upstream_only_repo/tools/release/verify_rebase_merge.sh"
chmod +x "$upstream_only_repo/tools/release/"*.sh
git -C "$upstream_only_repo" remote add upstream "https://github.com/selamy-labs/bazel-protobuf-bff-template.git"
git -C "$upstream_only_repo" config url."$upstream_source/.git".insteadOf "https://github.com/selamy-labs/bazel-protobuf-bff-template.git"

if ! git -C "$upstream_only_repo" rev-parse --verify --quiet "${upstream_candidate_sha}^{commit}" >/dev/null 2>&1; then
  expect_merge_in_repo "$upstream_only_repo" "merge_pr.sh fetches missing candidate head from upstream remote" 0 "selamy-labs-bot" 42 --match-head-commit "$upstream_candidate_sha"
  grep -qF "pr merge 42 --rebase --match-head-commit $upstream_candidate_sha --repo selamy-labs/bazel-protobuf-bff-template" "$gh_merge_log" || { echo "FAIL expected merge command for upstream remote in log"; failures=$((failures + 1)); }
else
  echo "FAIL upstream candidate commit unexpectedly present before fetch"; failures=$((failures + 1))
fi
rm -rf "$upstream_source" "$upstream_only_repo"

# Negative tests for SOL-R15-02 / BFF-RM15-02: strict --repo normalization and validation
# 1. Explicit non-GitHub URL (e.g. GitLab) is rejected with exit 1 before preflight
expect_merge_in_repo "$merge_repo" "merge_pr.sh fails closed on explicit GitLab URL" 1 "selamy-labs-bot" 42 --match-head-commit "$clean_candidate_sha" --repo https://gitlab.example.com/selamy-labs/bazel-protobuf-bff-template.git
grep -qF "not a valid GitHub repository selector" "$T/out" || { echo "FAIL expected not a valid selector error for GitLab URL"; failures=$((failures + 1)); }
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for explicit GitLab URL"; failures=$((failures + 1)); }

# 2. Explicit non-GitHub host selector is rejected with exit 1
expect_merge_in_repo "$merge_repo" "merge_pr.sh fails closed on explicit GitLab selector" 1 "selamy-labs-bot" 42 --match-head-commit "$clean_candidate_sha" --repo gitlab.example.com/selamy-labs/bazel-protobuf-bff-template
grep -qF "not a valid GitHub repository selector" "$T/out" || { echo "FAIL expected not a valid selector error for GitLab selector"; failures=$((failures + 1)); }
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for explicit GitLab selector"; failures=$((failures + 1)); }

# 3. Arbitrary invalid selector is rejected with exit 1
expect_merge_in_repo "$merge_repo" "merge_pr.sh fails closed on arbitrary invalid selector" 1 "selamy-labs-bot" 42 --match-head-commit "$clean_candidate_sha" --repo not-a-selector
grep -qF "not a valid GitHub repository selector" "$T/out" || { echo "FAIL expected not a valid selector error for arbitrary invalid selector"; failures=$((failures + 1)); }
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for arbitrary invalid selector"; failures=$((failures + 1)); }

# 4. Default invocation without --repo fails closed when local repo has only non-GitHub remotes
merge_repo_non_gh="$T/merge-repo-non-gh"
git init -q -b main "$merge_repo_non_gh"
git -C "$merge_repo_non_gh" remote add origin "https://gitlab.example.com/other-org/other-repo.git"
git -C "$merge_repo_non_gh" remote add fs "/tmp/fake-fs-path"
(cd "$merge_repo_non_gh" && printf 'initial\n' >a.txt && git add . && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="github-bot@selamy.dev" git commit -q -m "initial bot merge commit")
mkdir -p "$merge_repo_non_gh/tools/release"
cp "$allow_src" "$merge_repo_non_gh/tools/release/neutrality.allow"
cp "$neut_src" "$merge_repo_non_gh/tools/release/neutrality.sh"
cp "$vrm_src" "$merge_repo_non_gh/tools/release/verify_rebase_merge.sh"
chmod +x "$merge_repo_non_gh/tools/release/"*.sh
(cd "$merge_repo_non_gh" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q --allow-empty -m "clean candidate commit")
clean_non_gh_sha="$(git -C "$merge_repo_non_gh" rev-parse HEAD)"

expect_merge_in_repo "$merge_repo_non_gh" "merge_pr.sh fails closed on default invocation with only non-GitHub remotes" 1 "selamy-labs-bot" 42 --match-head-commit "$clean_non_gh_sha"
grep -qF "cannot resolve GitHub repository selector" "$T/out" || { echo "FAIL expected cannot resolve GitHub selector for non-gh default invocation"; failures=$((failures + 1)); }
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for non-gh default invocation"; failures=$((failures + 1)); }
rm -rf "$merge_repo_non_gh"

# 5. Verification repository mismatch: checkout's remotes do not match target --repo
mismatched_repo="$T/mismatched-repo"
git init -q -b main "$mismatched_repo"
git -C "$mismatched_repo" remote add origin "https://github.com/someone-else/different-repo.git"
(cd "$mismatched_repo" && printf 'initial\n' >a.txt && git add . && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="github-bot@selamy.dev" git commit -q -m "initial bot merge commit")
mkdir -p "$mismatched_repo/tools/release"
cp "$allow_src" "$mismatched_repo/tools/release/neutrality.allow"
cp "$neut_src" "$mismatched_repo/tools/release/neutrality.sh"
cp "$vrm_src" "$mismatched_repo/tools/release/verify_rebase_merge.sh"
chmod +x "$mismatched_repo/tools/release/"*.sh
(cd "$mismatched_repo" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q --allow-empty -m "clean candidate commit")
clean_mismatched_sha="$(git -C "$mismatched_repo" rev-parse HEAD)"

expect_merge_in_repo "$mismatched_repo" "merge_pr.sh fails closed when verification repo does not match target --repo" 1 "selamy-labs-bot" 42 --match-head-commit "$clean_mismatched_sha" --repo selamy-labs/bazel-protobuf-bff-template
grep -qF "has no remote matching target repository" "$T/out" || { echo "FAIL expected mismatched repo error in output"; failures=$((failures + 1)); }
[[ ! -s "$gh_merge_log" ]] || { echo "FAIL gh pr merge called for mismatched verification repo"; failures=$((failures + 1)); }
rm -rf "$mismatched_repo"

# 6. Fork origin setup: checkout has fork origin and upstream target
fork_repo="$T/fork-repo"
git init -q -b main "$fork_repo"
git -C "$fork_repo" remote add origin "https://github.com/fork-user/bazel-protobuf-bff-template.git"
git -C "$fork_repo" remote add upstream "https://github.com/selamy-labs/bazel-protobuf-bff-template.git"
(cd "$fork_repo" && printf 'initial\n' >a.txt && git add . && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="selamy-labs-bot" GIT_COMMITTER_EMAIL="github-bot@selamy.dev" git commit -q -m "initial bot merge commit")
mkdir -p "$fork_repo/tools/release"
cp "$allow_src" "$fork_repo/tools/release/neutrality.allow"
cp "$neut_src" "$fork_repo/tools/release/neutrality.sh"
cp "$vrm_src" "$fork_repo/tools/release/verify_rebase_merge.sh"
chmod +x "$fork_repo/tools/release/"*.sh
(cd "$fork_repo" && GIT_AUTHOR_NAME="claude-cir" GIT_AUTHOR_EMAIL="claude-cir@selamy.dev" GIT_COMMITTER_NAME="claude-cir" GIT_COMMITTER_EMAIL="claude-cir@selamy.dev" git commit -q --allow-empty -m "clean candidate commit")
clean_fork_sha="$(git -C "$fork_repo" rev-parse HEAD)"

# Form 6a: explicit target chooses upstream over fork origin
expect_merge_in_repo "$fork_repo" "merge_pr.sh prefers upstream over fork origin with explicit target" 0 "selamy-labs-bot" 42 --match-head-commit "$clean_fork_sha" --repo selamy-labs/bazel-protobuf-bff-template
grep -qF "pr merge 42 --rebase --match-head-commit $clean_fork_sha --repo selamy-labs/bazel-protobuf-bff-template" "$gh_merge_log" || { echo "FAIL expected merge command with upstream target in log"; failures=$((failures + 1)); }

# Form 6b: default invocation chooses upstream over fork origin
expect_merge_in_repo "$fork_repo" "merge_pr.sh prefers upstream over fork origin on default invocation" 0 "selamy-labs-bot" 42 --match-head-commit "$clean_fork_sha"
grep -qF "pr merge 42 --rebase --match-head-commit $clean_fork_sha --repo selamy-labs/bazel-protobuf-bff-template" "$gh_merge_log" || { echo "FAIL expected merge command with upstream target in log for default invocation"; failures=$((failures + 1)); }
rm -rf "$fork_repo"

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
