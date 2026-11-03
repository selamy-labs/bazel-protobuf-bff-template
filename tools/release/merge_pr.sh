#!/usr/bin/env bash
# Canonical merge gate script: ensures PRs are merged via rebase only by approved bot identities.
set -euo pipefail

is_approved_bot_login() {
  case "${1:-}" in
    selamy-labs-bot | claude-cir | "dependabot[bot]" | "github-actions[bot]") return 0 ;;
    *) return 1 ;;
  esac
}

pr=""
head_sha=""
repo=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --match-head-commit)
      [[ $# -ge 2 && -n "$2" ]] || { echo "merge_pr: --match-head-commit requires a sha" >&2; exit 1; }
      head_sha="$2"; shift 2 ;;
    --match-head-commit=*)
      head_sha="${1#--match-head-commit=}"
      [[ -n "$head_sha" ]] || { echo "merge_pr: --match-head-commit requires a sha" >&2; exit 1; }
      shift ;;
    --repo)
      [[ $# -ge 2 && -n "$2" ]] || { echo "merge_pr: --repo requires an owner/repo value" >&2; exit 1; }
      repo="$2"; shift 2 ;;
    --repo=*)
      repo="${1#--repo=}"
      [[ -n "$repo" ]] || { echo "merge_pr: --repo requires an owner/repo value" >&2; exit 1; }
      shift ;;
    -*)
      echo "merge_pr: unknown option $1" >&2; exit 1 ;;
    *)
      if [[ -z "$pr" ]]; then
        pr="$1"
      else
        echo "merge_pr: unexpected argument $1" >&2; exit 1
      fi
      shift ;;
  esac
done

[[ -n "$pr" ]] || { echo "merge_pr: missing PR number" >&2; exit 1; }
[[ -n "$head_sha" ]] || { echo "merge_pr: missing --match-head-commit <sha>" >&2; exit 1; }

caller_login="$(gh api user --jq .login 2>/dev/null || true)"
if [[ -z "$caller_login" ]] || ! is_approved_bot_login "$caller_login"; then
  echo "merge_pr: caller '${caller_login:-<empty>}' is not permitted to merge: merges must be executed by approved bot identities (selamy-labs-bot, claude-cir, dependabot[bot], or github-actions[bot])" >&2
  exit 1
fi

merge_args=("$pr" "--rebase" "--match-head-commit" "$head_sha")
if [[ -n "$repo" ]]; then
  merge_args+=("--repo" "$repo")
fi

exec gh pr merge "${merge_args[@]}"
