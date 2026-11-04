#!/usr/bin/env bash
# Deterministic merge-path ratchet: simulates a rebase-merge commit locally with the
# approved bot merger identity extracted from neutrality.allow, and runs neutrality scan.
set -euo pipefail

repo="."
allow=""
head_rev="HEAD"
base_rev=""
bot_committer=""
explicit_bot_committer=""
bot_name=""
explicit_bot_name=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      [[ $# -ge 2 && -n "$2" ]] || { echo "verify_rebase_merge: --repo requires a directory" >&2; exit 2; }
      repo="$2"; shift 2 ;;
    --repo=*)
      repo="${1#--repo=}"
      [[ -n "$repo" ]] || { echo "verify_rebase_merge: --repo requires a directory" >&2; exit 2; }
      shift ;;
    --allow)
      [[ $# -ge 2 && -n "$2" ]] || { echo "verify_rebase_merge: --allow requires a file" >&2; exit 2; }
      allow="$2"; shift 2 ;;
    --allow=*)
      allow="${1#--allow=}"
      [[ -n "$allow" ]] || { echo "verify_rebase_merge: --allow requires a file" >&2; exit 2; }
      shift ;;
    --head | --match-head-commit)
      [[ $# -ge 2 && -n "$2" ]] || { echo "verify_rebase_merge: $1 requires a commit sha" >&2; exit 2; }
      head_rev="$2"; shift 2 ;;
    --head=* | --match-head-commit=*)
      head_rev="${1#*=}"
      [[ -n "$head_rev" ]] || { echo "verify_rebase_merge: option requires a commit sha" >&2; exit 2; }
      shift ;;
    --base)
      [[ $# -ge 2 && -n "$2" ]] || { echo "verify_rebase_merge: --base requires a revision" >&2; exit 2; }
      base_rev="$2"; shift 2 ;;
    --base=*)
      base_rev="${1#--base=}"
      [[ -n "$base_rev" ]] || { echo "verify_rebase_merge: --base requires a revision" >&2; exit 2; }
      shift ;;
    --bot-committer)
      [[ $# -ge 2 && -n "$2" ]] || { echo "verify_rebase_merge: --bot-committer requires an email" >&2; exit 2; }
      explicit_bot_committer="$2"; shift 2 ;;
    --bot-committer=*)
      explicit_bot_committer="${1#--bot-committer=}"
      [[ -n "$explicit_bot_committer" ]] || { echo "verify_rebase_merge: --bot-committer requires an email" >&2; exit 2; }
      shift ;;
    --bot-name)
      [[ $# -ge 2 && -n "$2" ]] || { echo "verify_rebase_merge: --bot-name requires a name" >&2; exit 2; }
      explicit_bot_name="$2"; shift 2 ;;
    --bot-name=*)
      explicit_bot_name="${1#--bot-name=}"
      [[ -n "$explicit_bot_name" ]] || { echo "verify_rebase_merge: --bot-name requires a name" >&2; exit 2; }
      shift ;;
    -h | --help)
      echo "Usage: verify_rebase_merge.sh [--repo <dir>] [--allow <file>] [--head <rev>] [--base <rev>] [--bot-committer <email>]"
      exit 0 ;;
    -*)
      echo "verify_rebase_merge: unknown option $1" >&2; exit 2 ;;
    *)
      if [[ "$repo" == "." ]]; then
        repo="$1"
      else
        echo "verify_rebase_merge: unexpected argument $1" >&2; exit 2
      fi
      shift ;;
  esac
done

git -C "$repo" rev-parse --git-dir >/dev/null 2>&1 || {
  echo "verify_rebase_merge: '$repo' is not a git repository" >&2
  exit 2
}

repo_top="$(git -C "$repo" rev-parse --show-toplevel 2>/dev/null || pwd)"

if [[ -z "$allow" ]]; then
  if [[ -f "$repo_top/tools/release/neutrality.allow" ]]; then
    allow="$repo_top/tools/release/neutrality.allow"
  elif [[ -f "${BASH_SOURCE[0]%/*}/neutrality.allow" ]]; then
    allow="${BASH_SOURCE[0]%/*}/neutrality.allow"
  else
    echo "verify_rebase_merge: cannot find neutrality.allow; specify with --allow" >&2
    exit 2
  fi
fi

[[ -r "$allow" ]] || {
  echo "verify_rebase_merge: allowlist '$allow' cannot be read" >&2
  exit 2
}
allow="$(cd "$(dirname "$allow")" 2>/dev/null && pwd)/$(basename "$allow")"

neutrality_sh="tools/release/neutrality.sh"
[[ -f "$neutrality_sh" ]] || neutrality_sh="$repo_top/tools/release/neutrality.sh"
[[ -f "$neutrality_sh" ]] || neutrality_sh="${BASH_SOURCE[0]%/*}/neutrality.sh"
if [[ ! -f "$neutrality_sh" && -n "${RUNFILES_DIR:-}" ]]; then
  neutrality_sh="${RUNFILES_DIR}/_main/tools/release/neutrality.sh"
fi
if [[ ! -f "$neutrality_sh" && -n "${TEST_SRCDIR:-}" ]]; then
  neutrality_sh="${TEST_SRCDIR}/_main/tools/release/neutrality.sh"
fi

[[ -x "$neutrality_sh" ]] || {
  echo "verify_rebase_merge: cannot find executable neutrality.sh at $neutrality_sh" >&2
  exit 2
}

head_sha="$(git -C "$repo" rev-parse --verify --quiet "${head_rev}^{commit}")" || {
  echo "verify_rebase_merge: cannot resolve candidate head revision: $head_rev" >&2
  exit 2
}

if [[ -z "$base_rev" ]]; then
  if git -C "$repo" rev-parse --verify --quiet "origin/main^{commit}" >/dev/null 2>&1; then
    base_rev="origin/main"
  elif git -C "$repo" rev-parse --verify --quiet "main^{commit}" >/dev/null 2>&1; then
    base_rev="main"
  else
    base_rev="${head_sha}~1"
  fi
fi

base_sha="$(git -C "$repo" rev-parse --verify --quiet "${base_rev}^{commit}")" || {
  echo "verify_rebase_merge: cannot resolve base revision: $base_rev" >&2
  exit 2
}

# Derive bot merger identity from OBSERVED git history (most recent bot rebase-merge commit)
# rather than checking the allowlist against itself (BFF-RM12-03, SOL-R13-03, ARGON-BFF-R13-002).
# An explicit override must NOT bypass the observed-history requirement (SOL-R13-03).
observed_commit="$(git -C "$repo" log -n 100 --format='%cn%x1f%ce' "${base_sha:-HEAD}" 2>/dev/null | awk -F'\x1f' '($1 == "selamy-labs-bot" || $1 == "selamy-labs-bot[bot]") && !found {found = $1 "\x1f" $2} END {if (found) print found}')"
if [[ -z "$observed_commit" ]]; then
  observed_commit="$(git -C "$repo" log -n 100 --format='%cn%x1f%ce' --all 2>/dev/null | awk -F'\x1f' '($1 == "selamy-labs-bot" || $1 == "selamy-labs-bot[bot]") && !found {found = $1 "\x1f" $2} END {if (found) print found}')"
fi

if [[ -z "$observed_commit" ]]; then
  echo "verify_rebase_merge: no bot rebase-merge commit observed in history (expected selamy-labs-bot as on b2cd2b1f); cannot simulate merge" >&2
  exit 1
fi

observed_bot_name="${observed_commit%%$'\x1f'*}"
observed_bot_committer="${observed_commit#*$'\x1f'}"

if [[ -n "$explicit_bot_committer" && "$explicit_bot_committer" != "$observed_bot_committer" ]]; then
  echo "verify_rebase_merge: explicit --bot-committer '$explicit_bot_committer' does not match observed bot committer '$observed_bot_committer'" >&2
  exit 1
fi

if [[ -n "$explicit_bot_name" && "$explicit_bot_name" != "$observed_bot_name" ]]; then
  echo "verify_rebase_merge: explicit --bot-name '$explicit_bot_name' does not match observed bot name '$observed_bot_name'" >&2
  exit 1
fi

bot_name="$observed_bot_name"
bot_committer="$observed_bot_committer"

# Verify that the observed bot committer is permitted by the allowlist
if ! awk -v ce="$bot_committer" '$1 == "committer" && tolower($2 "@" $3) == tolower(ce) {found=1; exit} END {exit !found}' "$allow"; then
  echo "verify_rebase_merge: allowlist '$allow' does not permit observed bot merger committer identity $bot_name <$bot_committer>" >&2
  exit 1
fi

tree_sha="$(git -C "$repo" rev-parse "${head_sha}^{tree}")" || {
  echo "verify_rebase_merge: cannot resolve tree for $head_sha" >&2
  exit 2
}

author_name="$(git -C "$repo" log -1 --format='%an' "$head_sha")"
author_email="$(git -C "$repo" log -1 --format='%ae' "$head_sha")"
author_date="$(git -C "$repo" log -1 --format='%ad' "$head_sha")"
commit_msg="$(git -C "$repo" log -1 --format='%B' "$head_sha")"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

git clone --shared -q "$repo" "$work/scratch"

sim_sha="$(printf '%s\n' "$commit_msg" | \
  GIT_AUTHOR_NAME="$author_name" \
  GIT_AUTHOR_EMAIL="$author_email" \
  GIT_AUTHOR_DATE="$author_date" \
  GIT_COMMITTER_NAME="$bot_name" \
  GIT_COMMITTER_EMAIL="$bot_committer" \
  git -C "$work/scratch" commit-tree "$tree_sha" -p "$base_sha")"

[[ -n "$sim_sha" ]] || {
  echo "verify_rebase_merge: failed to create synthetic commit" >&2
  exit 1
}

git -C "$work/scratch" update-ref refs/heads/simulated-rebase-merge "$sim_sha"
git -C "$work/scratch" checkout -q refs/heads/simulated-rebase-merge

env -i PATH="$PATH" LC_ALL=C.UTF-8 HOME="${HOME:-/tmp}" "$neutrality_sh" "$work/scratch" --allow "$allow" --base "$base_sha" --commit "$sim_sha" --internal-simulation
