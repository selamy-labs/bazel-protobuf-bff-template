#!/usr/bin/env bash
# Installs the hooks with the pinned prek (spec 006 T012, FR-015). Run it with
#
#   <bazel> run @bff_template_quality//hooks:install [-- --weak-isolation] [--policy-base=<rev>]
#
# --policy-base stores the revision a push that escalates to `full` is compared with, as given, in local
# Git config. Nothing derives it, and naming it claims no review (spec 007 FR-015a).
#
# Keep prek responsible for installation, foreign-hook chaining, worktrees and core.hooksPath.
# Its generated marker is retained, but its PATH fallback is replaced by the small pin/input guard.
# Both the guard and the pinned binary live beside Git data, independently of cleaned Bazel outputs.
set -euo pipefail
guard="$(dirname "$(realpath "${BASH_SOURCE[0]}")")/guard.sh"
[[ -f "$guard" ]] || { echo "install: maintained hook guard is missing" >&2; exit 2; }
prek="$(realpath "$1")"
shift
isolation=full
policy_base=""
for arg in "$@"; do
  case "$arg" in
    --weak-isolation) isolation=weakened ;;
    --policy-base=?*) policy_base="${arg#--policy-base=}" ;;
    *) echo "install: unknown argument '$arg'" >&2; exit 2 ;;
  esac
done
cd "${BUILD_WORKSPACE_DIRECTORY:?run this with bazel run}"
common="$(git rev-parse --path-format=absolute --git-common-dir)"
bin="$common/bff-template/bin"
mkdir -p "$bin"
copy="$(mktemp "$bin/prek.XXXXXX")"
cp "$prek" "$copy"
chmod 755 "$copy"
mv "$copy" "$bin/prek"
read -r digest _ < <(sha256sum <"$bin/prek")
printf '%s\n' "$digest" >"$bin/prek.sha256.new"
mv "$bin/prek.sha256.new" "$bin/prek.sha256"
guard_copy="$(mktemp "$bin/hook-guard.XXXXXX")"
cp "$guard" "$guard_copy"
chmod 755 "$guard_copy"
mv "$guard_copy" "$bin/hook-guard"
"$bin/prek" install --hook-type pre-commit --hook-type pre-push
for stage in pre-commit pre-push; do
  hook="$(git rev-parse --path-format=absolute --git-path "hooks/$stage")"
  # Fail if the pinned generator changes its known shape; never silently retain a PATH fallback.
  if ! grep -qx '# ID: 182c10f181da4464a3eec51b83331688' "$hook" ||
      ! grep -qx "HERE=\"\$(cd \"\$(dirname \"\$0\")\" && pwd)\"" "$hook" ||
      ! grep -qx "exec \"\$PREK\" hook-impl --hook-dir \"\$HERE\" --script-version 4 --hook-type=$stage -- \"\$@\"" "$hook"; then
    printf '#!/bin/sh\necho "hook-guard: unsupported generated shim; reinstall with the maintained tool" >&2\nexit 2\n' >"$hook"
    chmod 755 "$hook"
    echo "install: unsupported pinned prek shim shape" >&2
    exit 2
  fi
  updated="$(mktemp "$hook.XXXXXX")"
  awk '/^# Check if the full path to prek is executable, otherwise fallback to PATH$/ { exit } { print }' "$hook" >"$updated"
  cat >>"$updated" <<'SH'
if [ ! -x "${PREK%/*}/hook-guard" ]; then
    echo "hook-guard: maintained guard is missing; reinstall hooks" >&2
    exit 2
fi
SH
  printf "exec \"\${PREK%%/*}/hook-guard\" \"\$PREK\" \"\$HERE\" %s \"\$@\"\n" "$stage" >>"$updated"
  chmod 755 "$updated"
  mv "$updated" "$hook"
done
git config bff-template.isolation "$isolation"
if [[ -n "$policy_base" ]]; then git config bff-template.policy-base "$policy_base"; fi
echo "install: hooks use $bin/prek (isolation: $isolation)"
