#!/usr/bin/env bash
# One lifecycle stage, as prek calls it (spec 006 T012):
#
#   tools/hooks/stage.sh <pre-commit|commit-msg|pre-push>
#
# The environment is scrubbed as tools/verify.sh scrubs it, keeping only the index Git named, because
# `git commit -a` and `git commit <path>` commit a temporary index. A hook-only isolation choice lives
# in Git configuration, set by //tools/hooks:install, so it cannot arrive from the environment.
set -euo pipefail
stage="${1:?usage: stage.sh <stage>}"
root="$(git rev-parse --show-toplevel)"
cd "$root"
common="$(git rev-parse --path-format=absolute --git-common-dir)"
state="$common/bff-template"
isolation="$(git config --get bff-template.isolation || echo full)"

keep=(
  "HOME=$HOME"
  "USER=${USER:-$(id -un)}"
  "PATH=/usr/bin:/bin"
  "TMPDIR=${TMPDIR:-/tmp}"
  "LC_ALL=C.UTF-8"
  "TERM=dumb"
  "GIT_CONFIG_GLOBAL=/dev/null"
  "GIT_CONFIG_SYSTEM=/dev/null"
  "GIT_TERMINAL_PROMPT=0"
)
for name in GIT_INDEX_FILE XDG_CACHE_HOME HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy \
  SSL_CERT_FILE SSL_CERT_DIR; do
  if [[ -n "${!name:-}" ]]; then keep+=("$name=${!name}"); fi
done

work="$(mktemp -d)"
trap 'rm -rf "${work:?}"' EXIT
flags=()
if [[ "$isolation" == weakened ]]; then flags+=(--config=weak-isolation); fi

# Building the runner is not a verdict on anything staged: a failure here says so, and exits with the
# runner's configuration code, not its rejection code.
if ! env -i "${keep[@]}" tools/bootstrap/bazel.sh run "${flags[@]}" "--script_path=$work/runner" \
  --noshow_progress --ui_event_filters=-info,-stdout,-stderr //:quality </dev/null >"$work/bootstrap.log" 2>&1; then
  tail -n 15 "$work/bootstrap.log" >&2
  echo "bootstrap ERROR: the quality runner did not build, so no check ran. This is not a verdict on" >&2
  echo "the staged content. Fix the build of the checkout, or bypass with care (docs/quality/hooks.md)." >&2
  exit 2
fi

arguments=("--stage=$stage" "--isolation=$isolation" "--receipt=$state/receipts/$stage.json")
if [[ "$stage" == pre-push ]]; then
  [[ -n "${PRE_COMMIT_TO_REF:-}" ]] || { echo "stage: pre-push needs PRE_COMMIT_TO_REF, which prek sets" >&2; exit 2; }
  arguments+=("--to=$PRE_COMMIT_TO_REF" "--from=${PRE_COMMIT_FROM_REF:-}")
fi
env -i "${keep[@]}" "$work/runner" "${arguments[@]}" </dev/null
