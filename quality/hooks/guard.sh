#!/usr/bin/env bash
# Spec006 T020/T021. Admission only; prek and the canonical stage still own the verdict.
set -euo pipefail
pin="${1:?pin}"
hook_dir="${2:?hook directory}"
stage="${3:?stage}"
shift 3
fail() { echo "hook-guard: $1; no stage verdict ran" >&2; exit 2; }
[[ -f "$pin" && -x "$pin" ]] || fail "recorded pinned prek is absent or not executable; reinstall hooks"
[[ -f "$pin.sha256" ]] || fail "recorded prek digest is missing; reinstall hooks"
expected="$(cat "$pin.sha256")"
[[ "$expected" =~ ^[0-9a-f]{64}$ ]] || fail "recorded prek digest is invalid; reinstall hooks"
read -r actual _ < <(sha256sum <"$pin")
[[ "$actual" == "$expected" ]] || fail "recorded pinned prek changed; reinstall hooks"
case "$stage" in
  pre-commit) exec "$pin" hook-impl --hook-dir "$hook_dir" --script-version 4 --hook-type=pre-commit -- "$@" </dev/null ;;
  pre-push) ;;
  *) fail "unsupported installed stage" ;;
esac
[[ $# -eq 2 ]] || fail "pre-push requires Git's original remote arguments"
[[ ! -t 0 ]] || fail "pre-push requires Git's original input"
packet="$(mktemp "${TMPDIR:-/tmp}/bff-pre-push.XXXXXXXX")"
trap 'rm -f "${packet:?}"' EXIT
head -c 1048577 >"$packet"
size="$(wc -c <"$packet")"
[[ "$size" -le 1048576 ]] || fail "pre-push input exceeds 1 MiB"
clean_size="$(tr -d '\000' <"$packet" | wc -c)"
[[ "$size" == "$clean_size" ]] || fail "pre-push input contains NUL"
head="$(git rev-parse --verify HEAD)"
zero="${head//?/0}"
source_count=0
# Match the literal-space packet grammar consumed by the pinned prek parser.
row_pattern='^([^[:space:]]+) ([0-9a-f]+) ([^[:space:]]+) ([0-9a-f]+)$'
while IFS= read -r row || [[ -n "$row" ]]; do
  [[ "$row" =~ $row_pattern ]] || fail "malformed pre-push row"
  local_ref="${BASH_REMATCH[1]}"
  local_oid="${BASH_REMATCH[2]}"
  remote_ref="${BASH_REMATCH[3]}"
  remote_oid="${BASH_REMATCH[4]}"
  [[ "$local_oid" =~ ^[0-9a-f]+$ && "$remote_oid" =~ ^[0-9a-f]+$ && ${#local_oid} -eq ${#head} && ${#remote_oid} -eq ${#head} ]] || fail "malformed pre-push object identity"
  git check-ref-format "$remote_ref" >/dev/null 2>&1 || fail "malformed pre-push destination"
  if [[ "$local_oid" == "$zero" ]]; then
    [[ "$local_ref" == '(delete)' && "$remote_oid" != "$zero" ]] || fail "malformed deletion row"
    continue
  fi
  source_count=$((source_count + 1))
  [[ "$source_count" -eq 1 ]] || fail "multiple source updates are unsupported; push one checked-out branch at a time"
  [[ "$remote_ref" == refs/heads/* ]] || fail "tag and non-branch updates are unsupported"
  [[ "$local_ref" == HEAD || "$local_ref" == refs/heads/* ]] || fail "only a checked-out branch or HEAD source is supported"
  [[ "$local_oid" == "$head" ]] || fail "source tip differs from checked-out HEAD"
  resolved="$(git rev-parse --verify "$local_ref^{commit}" 2>/dev/null)" || fail "source ref cannot be resolved"
  [[ "$resolved" == "$local_oid" ]] || fail "source ref differs from Git's declared object"
done <"$packet"
# Preserve every original byte for prek's legacy hook and selected stage; unlink the owned
# temporary name before exec, keeping only this open input descriptor until prek exits.
exec 0<"$packet"
rm -f "$packet"
trap - EXIT
exec "$pin" hook-impl --hook-dir "$hook_dir" --script-version 4 --hook-type=pre-push -- "$@"
