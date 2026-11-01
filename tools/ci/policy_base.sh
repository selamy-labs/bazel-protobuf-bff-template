#!/usr/bin/env bash
# Prints `sha=<policy base>` for GITHUB_OUTPUT, from values the workflow passes in the environment (spec
# 007 R6). Exit 11 means there is no base: the verifier then refuses by the same contract as any other
# run, so this script is never the place a base gets invented.
#
#   EVENT=<event name> PR_BASE=<sha> MERGE_BASE=<sha> PUSH_BEFORE=<sha> tools/ci/policy_base.sh
set -euo pipefail
refuse() { echo "policy-base: $1" >&2; exit 11; }
case "${EVENT:-}" in
  pull_request)
    [[ -n "${PR_BASE:-}" ]] || refuse "the event names no base"
    # A base that moved on is not an ancestor, so a pull request is judged against its fork point.
    base="$(git merge-base "$PR_BASE" HEAD)" || refuse "no merge base with $PR_BASE"
    ;;
  merge_group)
    base="${MERGE_BASE:-}"
    ;;
  push)
    base="${PUSH_BEFORE:-}"
    if [[ -z "$base" || -z "${base//0/}" ]]; then
      refuse "this push has no earlier commit: a repository's first push is its bootstrap, not a comparison"
    fi
    ;;
  *) refuse "unknown event '${EVENT:-}'" ;;
esac
[[ "$base" =~ ^[0-9a-f]{40}$ ]] || refuse "'$base' is not a commit"
echo "sha=$base"
