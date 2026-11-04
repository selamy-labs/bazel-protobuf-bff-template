#!/usr/bin/env bash
# Canonical merge gate script: ensures PRs are merged via rebase only by approved bot identities.
set -euo pipefail

is_approved_bot_login() {
  case "${1:-}" in
    selamy-labs-bot | claude-cir | "dependabot[bot]" | "github-actions[bot]") return 0 ;;
    *) return 1 ;;
  esac
}

parse_github_slug() {
  local input="$1"
  local slug=""
  if [[ "$input" =~ ^https?://github\.com/([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+?)(\.git)?$ ]]; then
    slug="${BASH_REMATCH[1]}"
  elif [[ "$input" =~ ^ssh://git@github\.com/([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+?)(\.git)?$ ]]; then
    slug="${BASH_REMATCH[1]}"
  elif [[ "$input" =~ ^git@github\.com:([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+?)(\.git)?$ ]]; then
    slug="${BASH_REMATCH[1]}"
  elif [[ "$input" =~ ^github\.com/([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+?)(\.git)?$ ]]; then
    slug="${BASH_REMATCH[1]}"
  elif [[ "$input" =~ ^([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+)$ ]]; then
    slug="${BASH_REMATCH[1]}"
    if [[ "$input" == *":"* || "$input" == *"@"* || "$input" == *"."*"/"* ]]; then
      return 1
    fi
  else
    return 1
  fi
  slug="${slug%.git}"
  local owner="${slug%%/*}"
  local repo_name="${slug#*/}"
  [[ -n "$owner" && -n "$repo_name" ]] || return 1
  printf '%s\n' "$slug"
  return 0
}

resolve_matching_github_remote() {
  local dir="$1"
  local target_slug="${2:-}"
  local r url slug

  if [[ -n "$target_slug" ]]; then
    local candidate_remotes=()
    if git -C "$dir" config --get remote.upstream.url >/dev/null 2>&1; then
      candidate_remotes+=("upstream")
    fi
    if git -C "$dir" config --get remote.origin.url >/dev/null 2>&1; then
      candidate_remotes+=("origin")
    fi
    for r in $(git -C "$dir" remote 2>/dev/null); do
      [[ "$r" == "upstream" || "$r" == "origin" ]] && continue
      candidate_remotes+=("$r")
    done

    for r in "${candidate_remotes[@]}"; do
      url="$(git -C "$dir" config --get "remote.$r.url" 2>/dev/null || true)"
      [[ -z "$url" ]] && continue
      if slug="$(parse_github_slug "$url" 2>/dev/null)"; then
        if [[ "$slug" == "$target_slug" ]]; then
          printf '%s %s\n' "$r" "$slug"
          return 0
        fi
      fi
    done
    return 1
  fi

  local candidate_remotes=()
  if git -C "$dir" config --get remote.upstream.url >/dev/null 2>&1; then
    candidate_remotes+=("upstream")
  fi
  if git -C "$dir" config --get remote.origin.url >/dev/null 2>&1; then
    candidate_remotes+=("origin")
  fi
  for r in $(git -C "$dir" remote 2>/dev/null); do
    [[ "$r" == "upstream" || "$r" == "origin" ]] && continue
    candidate_remotes+=("$r")
  done

  for r in "${candidate_remotes[@]}"; do
    url="$(git -C "$dir" config --get "remote.$r.url" 2>/dev/null || true)"
    [[ -z "$url" ]] && continue
    if slug="$(parse_github_slug "$url" 2>/dev/null)"; then
      printf '%s %s\n' "$r" "$slug"
      return 0
    fi
  done
  return 1
}

resolve_github_remote_and_slug() {
  resolve_matching_github_remote "$@"
}

resolve_github_slug() {
  local dir="$1"
  local target_slug="${2:-}"
  local remote_and_slug
  if remote_and_slug="$(resolve_matching_github_remote "$dir" "$target_slug")"; then
    printf '%s\n' "${remote_and_slug#* }"
    return 0
  fi
  return 1
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

local_repo="."
gh_repo=""
fetch_remote="origin"
script_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." 2>/dev/null && pwd || true)"

if [[ -n "$repo" && ( "$repo" == /* || "$repo" == ./* || "$repo" == ../* || "$repo" == "." || -d "$repo" ) ]]; then
  # Local filesystem path supplied as --repo: resolve GitHub selector from remote
  if [[ -d "$repo" ]] && git -C "$repo" rev-parse --git-dir >/dev/null 2>&1; then
    local_repo="$repo"
    if remote_and_slug="$(resolve_matching_github_remote "$local_repo")"; then
      gh_repo="${remote_and_slug#* }"
      fetch_remote="${remote_and_slug%% *}"
    else
      echo "merge_pr: cannot resolve GitHub repository selector [HOST/]OWNER/REPO from remote of '$repo'" >&2
      exit 1
    fi
  else
    echo "merge_pr: '$repo' is not a valid git repository" >&2
    exit 1
  fi
elif [[ -n "$repo" ]]; then
  # Explicit repository selector (owner/repo or GitHub URL) supplied
  if ! gh_repo="$(parse_github_slug "$repo")"; then
    echo "merge_pr: '$repo' is not a valid GitHub repository selector" >&2
    exit 1
  fi

  if git rev-parse --git-dir >/dev/null 2>&1; then
    local_repo="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
  elif [[ -n "$script_root" ]] && git -C "$script_root" rev-parse --git-dir >/dev/null 2>&1; then
    local_repo="$script_root"
  fi

  git -C "$local_repo" rev-parse --git-dir >/dev/null 2>&1 || {
    echo "merge_pr: cannot locate a valid local git repository to run pre-merge verification" >&2
    exit 1
  }

  if remote_and_slug="$(resolve_matching_github_remote "$local_repo" "$gh_repo")"; then
    fetch_remote="${remote_and_slug%% *}"
  else
    echo "merge_pr: verification repository '$local_repo' has no remote matching target repository '$gh_repo'" >&2
    exit 1
  fi
else
  # Default context: determine local repo and resolve GitHub remote
  if git rev-parse --git-dir >/dev/null 2>&1; then
    local_repo="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
  elif [[ -n "$script_root" ]] && git -C "$script_root" rev-parse --git-dir >/dev/null 2>&1; then
    local_repo="$script_root"
  fi

  git -C "$local_repo" rev-parse --git-dir >/dev/null 2>&1 || {
    echo "merge_pr: cannot locate a valid local git repository to run pre-merge verification" >&2
    exit 1
  }

  if remote_and_slug="$(resolve_matching_github_remote "$local_repo")"; then
    gh_repo="${remote_and_slug#* }"
    fetch_remote="${remote_and_slug%% *}"
  else
    echo "merge_pr: cannot resolve GitHub repository selector [HOST/]OWNER/REPO from remote of '$local_repo'" >&2
    exit 1
  fi
fi

# If candidate head commit is not available locally, attempt to fetch it before preflight
if ! git -C "$local_repo" rev-parse --verify --quiet "${head_sha}^{commit}" >/dev/null 2>&1; then
  git -C "$local_repo" fetch -q "$fetch_remote" "pull/$pr/head" 2>/dev/null || \
  git -C "$local_repo" fetch -q "$fetch_remote" "$head_sha" 2>/dev/null || \
  git -C "$local_repo" fetch -q "https://github.com/${gh_repo}.git" "$head_sha" 2>/dev/null || true
fi

verify_rebase_cmd="${BASH_SOURCE[0]%/*}/verify_rebase_merge.sh"
[[ -f "$verify_rebase_cmd" ]] || verify_rebase_cmd="$local_repo/tools/release/verify_rebase_merge.sh"
if [[ ! -x "$verify_rebase_cmd" ]]; then
  echo "merge_pr: pre-merge verifier '$verify_rebase_cmd' is missing or not executable" >&2
  exit 1
fi

"$verify_rebase_cmd" --repo "$local_repo" --match-head-commit "$head_sha" || {
  echo "merge_pr: simulated rebase-merge neutrality check failed for candidate head $head_sha" >&2
  exit 1
}

merge_args=("$pr" "--rebase" "--match-head-commit" "$head_sha" "--repo" "$gh_repo")

(cd "$local_repo" && exec gh pr merge "${merge_args[@]}")
