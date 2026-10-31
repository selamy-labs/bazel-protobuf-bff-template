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
  .github/actionlint.yaml .buildifier.json sub/.bazelrc .bazeliskrc api/buf.yaml buf.work.yaml api/buf.gen.yaml buf.lock; do
  printf '%s\0' "$path" >"$work/bad"
  if out="$("$policy" "$work/bad")"; then
    echo "FAIL $path accepted" >&2
    failures=$((failures + 1))
  elif ! grep -qF "implicit-config $path:1:" <<<"$out"; then
    echo "FAIL $path diagnostic: $out" >&2
    failures=$((failures + 1))
  fi
done
rc_policy=tools/policy/bazelrc_policy.sh
allow=tools/policy/bazelrc.allow
"$rc_policy" .bazelrc "$allow" 2>/dev/null || { echo "FAIL the real .bazelrc was rejected" >&2; failures=$((failures + 1)); }
rejects_rc() { # rejects_rc <label> <sed expression or appended line>
  cp .bazelrc "$work/rc"
  sed -i "$2" "$work/rc"
  if cmp -s .bazelrc "$work/rc"; then
    echo "FAIL $1: mutation changed nothing" >&2
    failures=$((failures + 1))
  elif "$rc_policy" "$work/rc" "$allow" 2>/dev/null; then
    echo "FAIL $1: accepted" >&2
    failures=$((failures + 1))
  fi
}
rejects_rc "network flag removed" '/sandbox_default_allow_network/d'
rejects_rc "strict env removed" '/incompatible_strict_action_env/d'
rejects_rc "sandbox weakened" 's/--spawn_strategy=linux-sandbox/--spawn_strategy=local/'
rejects_rc "tag filter added" "\$a test --test_tag_filters=-slow"
rejects_rc "flaky retries added" "\$a test --flaky_test_attempts=5"
rejects_rc "lockfile relaxed" 's/--lockfile_mode=error/--lockfile_mode=update/'
rejects_rc "import added" "\$a try-import /etc/hostile.bazelrc"

[[ $failures -eq 0 ]] && echo "ok   implicit-config, bazelrc-policy"
exit "$failures"
