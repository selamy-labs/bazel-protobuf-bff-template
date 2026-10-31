#!/usr/bin/env bash
# Witnesses for the implicit-configuration ban (specs/002-hermetic-skeleton T026).
set -euo pipefail
policy="$1"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
printf '%s\0' .bazelrc _typos.toml .gitleaks.toml MODULE.bazel docs/typos-notes.md >"$work/good"
"$policy" "$work/good" >/dev/null || { echo "FAIL canonical configs rejected" >&2; failures=1; }
for path in .shellcheckrc sub/.shellcheckrc sub/.typos.toml typos.toml sub/_typos.toml sub/.gitleaks.toml \
  .gitleaksignore sub/.gitleaksignore .gitleaksbaseline .github/zizmor.yml zizmor.yml .zizmor.yaml \
  .github/actionlint.yaml .buildifier.json sub/.bazelrc .bazeliskrc; do
  printf '%s\0' "$path" >"$work/bad"
  if out="$("$policy" "$work/bad")"; then
    echo "FAIL $path accepted" >&2
    failures=$((failures + 1))
  elif ! grep -qF "implicit-config $path:1:" <<<"$out"; then
    echo "FAIL $path diagnostic: $out" >&2
    failures=$((failures + 1))
  fi
done
[[ $failures -eq 0 ]] && echo "ok   implicit-config"
exit "$failures"
