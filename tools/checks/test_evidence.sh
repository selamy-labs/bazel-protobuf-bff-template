#!/usr/bin/env bash
# Judges a `bazel test` log rather than its exit code (specs/002-hermetic-skeleton T032).
#
#   test_evidence.sh inventory <log> <required list>  the tests that passed are exactly the list
#   test_evidence.sh sandbox <log> <full|weakened>    actions really ran in the claimed sandbox
#
# A test tagged manual, filtered out or incompatible never appears as PASSED, so it fails here
# although Bazel exits 0.
set -euo pipefail
mode="$1"
log="$2"
case "$mode" in
  inventory)
    required="$(grep -v '^#' "$3" | LC_ALL=C sort)"
    passed="$(awk '$2 == "PASSED" {print $1}' "$log" | LC_ALL=C sort -u)"
    if [[ "$passed" != "$required" ]]; then
      echo "test-inventory: passed tests differ from tools/checks/required_tests.txt (< required, > passed):" >&2
      diff <(echo "$required") <(echo "$passed") >&2 || true
      exit 1
    fi
    ;;
  sandbox)
    summary="$(grep -E '^INFO: [0-9]+ process(es)?: ' "$log" | tail -1)"
    [[ -n "$summary" ]] || { echo "sandbox-evidence: Bazel printed no process summary" >&2; exit 1; }
    if grep -qE '[0-9]+ (local|standalone)\b' <<<"$summary"; then
      echo "sandbox-evidence: actions ran outside a sandbox: $summary" >&2
      exit 1
    fi
    case "$3" in
      full) want='[0-9]+ (linux-sandbox|darwin-sandbox)\b' ;;
      weakened) want='[0-9]+ processwrapper-sandbox\b' ;;
      *) echo "sandbox-evidence: unknown label '$3'" >&2; exit 2 ;;
    esac
    grep -qE "$want" <<<"$summary" || { echo "sandbox-evidence: '$3' not shown by: $summary" >&2; exit 1; }
    ;;
  *)
    echo "test-evidence: unknown mode '$mode'" >&2
    exit 2
    ;;
esac
