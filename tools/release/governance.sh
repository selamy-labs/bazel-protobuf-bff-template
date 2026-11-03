#!/usr/bin/env bash
# The repository settings as a reviewed plan (spec 010 FR-009, FR-009a): format, validate, the pinned graph and
# the mock tests, with no reachable network and no credentials. The provider comes from a mirror assembled
# from the pinned zip and the lock's hash, so nothing is fetched. Nothing here applies anything.
#
#   governance.sh <tofu> <provider zip> <config dir> [<ci policy json> <workflow yml>] [--event <name>] [--event-path <file>]
#
# Exit 0 passed, 1 a finding, 11 nothing could be judged.
set -euo pipefail
refuse() { echo "governance: $1" >&2; exit 11; }

is_approved_bot_login() {
  case "${1:-}" in
    selamy-labs-bot | claude-cir | "dependabot[bot]" | "github-actions[bot]") return 0 ;;
    *) return 1 ;;
  esac
}

if [[ "${1:-}" == "--merge-pr" ]]; then
  shift
  merge_pr_helper="${0%/*}/merge_pr.sh"
  if [[ ! -x "$merge_pr_helper" ]]; then
    echo "governance: canonical merge helper merge_pr.sh is missing or not executable" >&2
    exit 1
  fi
  exec "$merge_pr_helper" "$@"
fi

if [[ "${1:-}" == "--verify-merge-actor" ]]; then
  shift
  caller_login="${1:-$(gh api user --jq .login 2>/dev/null || true)}"
  if [[ -z "$caller_login" ]] || ! is_approved_bot_login "$caller_login"; then
    echo "governance: caller '${caller_login:-<empty>}' is not permitted to merge: merges must be executed by approved bot identities (selamy-labs-bot, claude-cir, dependabot[bot], or github-actions[bot])" >&2
    exit 1
  fi
  echo "governance: caller '$caller_login' is an approved bot identity"
  exit 0
fi

event="${GITHUB_EVENT_NAME:-}"
event_path="${GITHUB_EVENT_PATH:-}"
explicit_event_path=0
positional=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --event)
      [[ $# -ge 2 && -n "$2" ]] || refuse "--event requires a value"
      event="$2"; shift 2 ;;
    --event=*)
      event="${1#--event=}"
      [[ -n "$event" ]] || refuse "--event requires a value"
      shift ;;
    --event-path)
      [[ $# -ge 2 && -n "$2" ]] || refuse "--event-path requires a file"
      event_path="$2"; explicit_event_path=1; shift 2 ;;
    --event-path=*)
      event_path="${1#--event-path=}"
      [[ -n "$event_path" ]] || refuse "--event-path requires a file"
      explicit_event_path=1; shift ;;
    *)
      positional+=("$1"); shift ;;
  esac
done

if [[ $explicit_event_path -eq 1 && -z "$event" ]]; then
  event="pull_request"
fi

if [[ $explicit_event_path -eq 1 && ! -r "$event_path" ]]; then
  refuse "event path cannot be read: $event_path"
fi

[[ ${#positional[@]} -ge 3 ]] || refuse "usage: governance.sh <tofu> <provider zip> <config dir> [<ci policy json> <workflow yml>] [--event <name>] [--event-path <file>]"
tofu="${positional[0]}"
zip="${positional[1]}"
config="${positional[2]}"
policy="${positional[3]:-}"
workflow="${positional[4]:-}"

# PR/merge_group actor ratchet: inspect author/actor from event payload and environment
if [[ "$event" == "pull_request" || "$event" == "pull_request_target" || "$event" == "merge_group" ]]; then
  if [[ -n "${GITHUB_ACTOR:-}" ]] && ! is_approved_bot_login "$GITHUB_ACTOR"; then
    echo "governance: GITHUB_ACTOR '${GITHUB_ACTOR}' is not permitted: events must be opened and merged by approved bot identities (selamy-labs-bot, claude-cir, dependabot[bot], or github-actions[bot])" >&2
    exit 1
  fi
  if [[ -n "${GITHUB_RUN_ID:-}" && -z "${GITHUB_ACTOR:-}" ]]; then
    echo "governance: GITHUB_ACTOR is required when running under GitHub Actions CI (GITHUB_RUN_ID is set)" >&2
    exit 1
  fi
  if [[ -n "${GITHUB_RUN_ID:-}" && -z "$event_path" ]]; then
    echo "governance: GITHUB_EVENT_PATH is required when running under GitHub Actions CI (GITHUB_RUN_ID is set)" >&2
    exit 1
  fi

  if [[ -n "$event_path" ]]; then
    if [[ ! -r "$event_path" ]]; then
      echo "governance: event path cannot be read: $event_path" >&2
      exit 1
    fi

    event_user=""
    parse_ok=0
    if command -v jq >/dev/null 2>&1; then
      if [[ "$event" == "merge_group" ]]; then
        if event_user_raw="$(jq -e -r '.merge_group.actor.login // .merge_group.head_commit.author.username // .sender.login // .actor.login // empty' "$event_path" 2>/dev/null)"; then
          if [[ -n "$event_user_raw" && "$event_user_raw" != "null" ]]; then
            event_user="$event_user_raw"
            parse_ok=1
          fi
        fi
      else
        if event_user_raw="$(jq -e -r '.pull_request.user.login // empty' "$event_path" 2>/dev/null)"; then
          if [[ -n "$event_user_raw" && "$event_user_raw" != "null" ]]; then
            event_user="$event_user_raw"
            parse_ok=1
          fi
        fi
      fi
    elif command -v python3 >/dev/null 2>&1; then
      if event_user_raw="$(python3 -c 'import json, sys
try:
    with open(sys.argv[1], "r", encoding="utf-8") as f:
        d = json.load(f)
    ev = sys.argv[2]
    if ev == "merge_group":
        mg = d.get("merge_group", {})
        login = (
            (mg.get("actor", {}) or {}).get("login")
            or (mg.get("head_commit", {}).get("author", {}) or {}).get("username")
            or (d.get("sender", {}) or {}).get("login")
            or (d.get("actor", {}) or {}).get("login")
        )
    else:
        login = (d.get("pull_request", {}).get("user", {}) or {}).get("login")
    if login and isinstance(login, str):
        print(login)
        sys.exit(0)
    sys.exit(1)
except Exception:
    sys.exit(1)
' "$event_path" "$event" 2>/dev/null)"; then
        event_user="$event_user_raw"
        parse_ok=1
      fi
    fi

    if [[ $parse_ok -ne 1 || -z "$event_user" ]]; then
      echo "governance: cannot extract valid non-empty user login from event payload $event_path" >&2
      exit 1
    elif ! is_approved_bot_login "$event_user"; then
      echo "governance: $event user '$event_user' is not permitted: events must be opened and merged by approved bot identities (selamy-labs-bot, claude-cir, dependabot[bot], or github-actions[bot])" >&2
      exit 1
    fi
  fi
fi

[[ -x "$tofu" ]] || refuse "no tofu at $tofu"
[[ -f "$zip" ]] || refuse "no provider zip at $zip"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
hash="$(grep -oE 'h1:[A-Za-z0-9+/=]+' "$config/.terraform.lock.hcl" | head -1)"
[[ -n "$hash" ]] || refuse "the lock names no h1 hash"
mirror="$work/mirror/registry.opentofu.org/integrations/github"
mkdir -p "$mirror"
cp "$zip" "$mirror/terraform-provider-github_6.13.0_linux_amd64.zip"
printf '{"versions":{"6.13.0":{}}}\n' >"$mirror/index.json"
printf '{"archives":{"linux_amd64":{"url":"terraform-provider-github_6.13.0_linux_amd64.zip","hashes":["%s"]}}}\n' "$hash" >"$mirror/6.13.0.json"
cat >"$work/cli.tfrc" <<CLI
provider_installation {
  filesystem_mirror {
    path    = "$work/mirror"
    include = ["integrations/github"]
  }
}
CLI
cp -rL "$config" "$work/config"
export HOME="$work/home" TF_PLUGIN_CACHE_DIR="" TF_CLI_CONFIG_FILE="$work/cli.tfrc" HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 NO_PROXY="" GITHUB_TOKEN="" GH_TOKEN="" TF_INPUT=0
mkdir -p "$HOME"
rm -rf "$work/config/.terraform"
"$tofu" -chdir="$work/config" fmt -check -recursive >/dev/null || { echo "governance: the configuration is not formatted"; exit 1; }
# The declaration is read structurally, not counted by file name: no JSON configuration, no module, no import, no
# data source, exactly the two resources, and then the real graph that tofu builds from the loaded configuration.
if find "$work/config" -name '*.tf.json' -o -name '*.tofu.json' | grep -q .; then echo "governance: JSON configuration is not accepted here"; exit 1; fi
! grep -rqE '^\s*(import|data|module) ' "$work/config"/*.tf || { echo "governance: an import block, a data source or a module is present"; exit 1; }
[[ "$(cat "$work/config"/*.tf | grep -cE '^resource ')" == 2 ]] || { echo "governance: the resource graph is not the two declared resources"; exit 1; }
"$tofu" -chdir="$work/config" init -input=false -lockfile=readonly >/dev/null || refuse "init did not complete from the mirror"
"$tofu" -chdir="$work/config" validate >/dev/null || { echo "governance: the configuration is not valid"; exit 1; }
graph="$("$tofu" -chdir="$work/config" graph 2>/dev/null)" || refuse "the graph could not be built"
nodes="$(grep -oE '"[a-z_]+\.[A-Za-z0-9_-]+( \(expand\))?"' <<<"$graph" | grep -E '^"github_' | sed 's/ (expand)//' | LC_ALL=C sort -u | tr -d '"' | tr '\n' ' ')"
[[ "$nodes" == "github_repository.template github_repository_ruleset.main " ]] || { echo "governance: the loaded graph holds [$nodes], not the two declared resources"; exit 1; }
grep -qE '^\s*owner\s*=\s*var\.owner\s*$' "$work/config/main.tf" || { echo "governance: the provider is not bound to the pinned owner"; exit 1; }
if ! grep -Eq '^\s*allow_rebase_merge\s*=\s*true\s*$' "$work/config/main.tf" || \
   ! grep -Eq '^\s*allow_squash_merge\s*=\s*false\s*$' "$work/config/main.tf" || \
   ! grep -Eq '^\s*allow_merge_commit\s*=\s*false\s*$' "$work/config/main.tf"; then
  echo "governance: repository must enforce rebase-only merge"
  exit 1
fi
if ! grep -Eq '^\s*allowed_merge_methods\s*=\s*\["rebase"\]\s*$' "$work/config/main.tf"; then
  echo "governance: ruleset pull_request must declare allowed_merge_methods = [\"rebase\"]"
  exit 1
fi
if ! grep -Eq '^\s*required_linear_history\s*=\s*true\s*$' "$work/config/main.tf"; then
  echo "governance: ruleset must declare required_linear_history = true"
  exit 1
fi

allow_file="${0%/*}/neutrality.allow"
if [[ ! -f "$allow_file" ]]; then
  allow_file="tools/release/neutrality.allow"
fi
if [[ ! -f "$allow_file" && -n "${RUNFILES_DIR:-}" ]]; then
  allow_file="${RUNFILES_DIR}/_main/tools/release/neutrality.allow"
fi
if [[ ! -f "$allow_file" && -n "${TEST_SRCDIR:-}" ]]; then
  allow_file="${TEST_SRCDIR}/_main/tools/release/neutrality.allow"
fi
expected_c_pat="$(awk '
  $1 == "committer" {
    email = $2 "@" $3
    gsub(/[\.+]/, "\\\\&", email)
    if (c == "") c = email; else c = c "|" email
  }
  END {
    claude = "claude-cir@selamy\\.dev"
    if (c == "") c = claude; else c = c "|" claude
    print "^(" c ")$"
  }
' "$allow_file")"
expected_a_pat="^claude-cir@selamy\\.dev$"

tofu_test_log="$work/tofu_test.json"
tofu_test_rc=0
"$tofu" -chdir="$work/config" test -json -verbose >"$tofu_test_log" 2>&1 || tofu_test_rc=$?

c_op="" c_neg="" c_pat="" a_op="" a_neg="" a_pat="" has_patterns=0

if command -v jq >/dev/null 2>&1; then
  eval "$(jq -r '
    [select(.test_plan != null) | .test_plan.planned_values.root_module.resources[]? | select(.address == "github_repository_ruleset.main") | .values.rules[0] | select(. != null)] | .[0] // empty |
    @sh "has_patterns=1; c_op=\(.committer_email_pattern[0].operator // ""); c_neg=\(.committer_email_pattern[0].negate | tostring); c_pat=\(.committer_email_pattern[0].pattern // ""); a_op=\(.commit_author_email_pattern[0].operator // ""); a_neg=\(.commit_author_email_pattern[0].negate | tostring); a_pat=\(.commit_author_email_pattern[0].pattern // "")"
  ' "$tofu_test_log" 2>/dev/null || true)"
elif command -v python3 >/dev/null 2>&1; then
  eval "$(python3 -c '
import json, sys, shlex
has_patterns = 0
c_op = c_neg = c_pat = a_op = a_neg = a_pat = ""
try:
    with open(sys.argv[1], "r", encoding="utf-8") as f:
        for line in f:
            try:
                d = json.loads(line)
                tp = d.get("test_plan")
                if not tp: continue
                resources = ((tp.get("planned_values") or {}).get("root_module") or {}).get("resources", [])
                for r in resources:
                    if r.get("address") == "github_repository_ruleset.main":
                        rules = (r.get("values") or {}).get("rules", [])
                        if rules:
                            rule = rules[0]
                            c = (rule.get("committer_email_pattern") or [{}])[0]
                            a = (rule.get("commit_author_email_pattern") or [{}])[0]
                            has_patterns = 1
                            c_op = c.get("operator") or ""
                            c_neg = str(c.get("negate", "")).lower()
                            c_pat = c.get("pattern") or ""
                            a_op = a.get("operator") or ""
                            a_neg = str(a.get("negate", "")).lower()
                            a_pat = a.get("pattern") or ""
                            break
                if has_patterns: break
            except Exception:
                pass
except Exception:
    pass
print(f"has_patterns={has_patterns}; c_op={shlex.quote(c_op)}; c_neg={shlex.quote(c_neg)}; c_pat={shlex.quote(c_pat)}; a_op={shlex.quote(a_op)}; a_neg={shlex.quote(a_neg)}; a_pat={shlex.quote(a_pat)}")
' "$tofu_test_log" 2>/dev/null || true)"
fi

if [[ "$has_patterns" -ne 1 || "$c_op" != "regex" || "$c_neg" != "false" || "$c_pat" != "$expected_c_pat" || \
      "$a_op" != "regex" || "$a_neg" != "false" || "$a_pat" != "$expected_a_pat" ]]; then
  echo "governance: ruleset must declare exact committer_email_pattern and commit_author_email_pattern"
  exit 1
fi
# The required status check must be the job that the workflow and the CI-parity policy call the gate (spec 010
# FR-009b). A context naming no job would make the protection require a check that no run ever reports, so the
# three names are related here, by the policy's top-level key and the workflow's job name under that id.
[[ -f "$policy" && -f "$workflow" ]] || refuse "the CI policy and the workflow are needed to relate the required check"
context="$(sed -nE 's/^\s*context\s*=\s*"([^"]+)"\s*$/\1/p' "$work/config/main.tf" | LC_ALL=C sort -u)"
[[ -n "$context" && "$(printf '%s\n' "$context" | wc -l)" -eq 1 ]] || { echo "governance: the ruleset must require exactly one status check context"; exit 1; }
gate="$(sed -nE 's/^  "gate": "([^"]+)",?$/\1/p' "$policy")"
[[ -n "$gate" ]] || refuse "the CI policy names no gate job"
jobname="$(awk -v id="$gate" '$0 == "  " id ":" {inside = 1; next} inside && /^  [A-Za-z0-9_-]+:$/ {inside = 0} inside && /^    name: / {sub(/^    name: /, ""); print; exit}' "$workflow")"
[[ -n "$jobname" ]] || { echo "governance: the workflow has no job $gate with a name"; exit 1; }
[[ "$context" == "$gate" && "$context" == "$jobname" ]] || { echo "governance: the ruleset requires the status check \"$context\", but the gate job is \"$gate\", named \"$jobname\""; exit 1; }
if [[ "$tofu_test_rc" -ne 0 ]]; then
  echo "governance: a mock test failed"
  exit 1
fi
echo "governance: one target, formatted, valid, two resources, mock tests passed, offline"
