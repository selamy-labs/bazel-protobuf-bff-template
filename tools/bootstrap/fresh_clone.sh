#!/usr/bin/env bash
# Fresh-clone witness (specs/002-hermetic-skeleton FR-011, SC-001, T028, T030).
#
#   tools/bootstrap/fresh_clone.sh [--weak-isolation] [--hostile]
#
# Clones HEAD into an empty directory whose path contains spaces and regular-expression
# metacharacters, with an empty HOME and only /usr/bin:/bin on PATH, so no Bazel, Bazelisk or uv
# installation, cache, credential or Git configuration of the author's is visible.
#
# --hostile also plants rc files and environment settings that would break or weaken the run, adds
# a real defect and requires the verifier to fail for that reason alone, then removes the defect
# and requires a pass.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
revision="$(git -C "$root" rev-parse HEAD)"
verify_args=()
hostile=false
for arg in "$@"; do
  case "$arg" in
    --weak-isolation) verify_args+=(--weak-isolation) ;;
    --hostile) hostile=true ;;
    *) echo "fresh-clone: unknown argument '$arg'" >&2; exit 2 ;;
  esac
done

work="$(mktemp -d)"
home="$work/home"
clone="$work/fresh [clone] (a|b)* copy"
mkdir -p "$home"
trap '(cd "$clone" 2>/dev/null && env -i HOME="$home" PATH=/usr/bin:/bin tools/bootstrap/bazel.sh shutdown) >/dev/null 2>&1; chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT

git clone --quiet --no-local "file://$root" "$clone"
git -C "$clone" checkout --quiet "$revision"

for tool in bazel bazelisk uv uvx go node buf shellcheck; do
  if env -i PATH=/usr/bin:/bin sh -c "command -v $tool" >/dev/null 2>&1; then
    echo "fresh-clone: note: $tool is on the controlled PATH; the verifier must still not depend on it"
  fi
done

environment=(env -i HOME="$home" PATH=/usr/bin:/bin USER="${USER:-$(id -un)}" TERM=dumb)
if [[ -n "${HTTPS_PROXY:-}" ]]; then environment+=(HTTPS_PROXY="$HTTPS_PROXY"); fi

if $hostile; then
  echo 'common --option_that_only_a_hostile_rc_would_set' >"$home/.bazelrc"
  echo 'disable=all' >"$home/.shellcheckrc"
  printf '[safe]\n\tdirectory = *\n[core]\n\texcludesFile = %s/ignored\n' "$home" >"$home/.gitconfig"
  echo '*.sh' >"$home/ignored"
  environment+=(BAZELRC="$home/.bazelrc" SHELLCHECK_OPTS=--exclude=SC2086 JAVA_TOOL_OPTIONS=-Dhostile=1
    USE_BAZEL_VERSION=1.0.0 GITLEAKS_CONFIG=/dev/null)
  cat >"$clone/tools/defect.sh" <<'SH'
#!/bin/sh
echo $1
SH
  rc=0
  (cd "$clone" && "${environment[@]}" tools/verify.sh "${verify_args[@]}" </dev/null) >"$work/log" 2>&1 || rc=$?
  cat "$work/log"
  if [[ $rc -eq 0 ]]; then
    echo "fresh-clone: hostile settings hid the planted defect" >&2
    exit 1
  fi
  grep -qE '^FAILED +shellcheck' "$work/log" || { echo "fresh-clone: did not fail on the planted defect" >&2; exit 1; }
  grep -q 'SC2086' "$work/log" || { echo "fresh-clone: shellcheck did not report the planted defect" >&2; exit 1; }
  if grep -E '^(FAILED|TIMED_OUT|ERROR|NOT_RUN) ' "$work/log" | grep -qvE '^FAILED +shellcheck'; then
    echo "fresh-clone: a check other than the planted defect failed" >&2
    exit 1
  fi
  rm "$clone/tools/defect.sh"
fi

rc=0
(cd "$clone" && "${environment[@]}" tools/verify.sh "${verify_args[@]}" </dev/null) || rc=$?
echo "fresh-clone: revision $revision, exit $rc"
cat "$clone/.verify/receipt.json" 2>/dev/null || echo "fresh-clone: no receipt was written" >&2
exit "$rc"
