#!/usr/bin/env bash
# Spec006 T020/T021: real Git/prek adapter admission; not product policy qualification.
set -euo pipefail
installer="$(realpath "${1:?installer}")"
prek="$(realpath "${2:?pinned prek}")"
work="$(mktemp -d)"
trap 'rm -rf "${work:?}"' EXIT
root="$work/repository with spaces"
remote="$work/remote.git"
mkdir "$root" "$work/decoy"
git init -q --initial-branch=main "$root"
git init -q --bare "$remote"
cd "$root"
git config user.name Fixture
git config user.email fixture@example.invalid
git config commit.gpgsign false
git remote add origin "$remote"
cat >record-stage.sh <<'SH'
#!/usr/bin/env bash
set -eu
printf 'stage:%s\n' "${PRE_COMMIT_TO_REF:-commit}" >>"$(git rev-parse --git-common-dir)/stage-trace"
SH
chmod +x record-stage.sh
cat >prek.toml <<'TOML'
[[repos]]
repo = "local"
hooks = [
    { id = "fixture", name = "fixture", language = "system", entry = "./record-stage.sh", stages = ["pre-commit", "pre-push"], pass_filenames = false, always_run = true },
]
TOML
printf 'fixture\n' >README.md
git add .
git commit -qm seed
base="$(git rev-parse HEAD)"
git branch delete-only
git branch delete-mixed
git tag delete-tag
git push -q origin main delete-only delete-mixed refs/tags/delete-tag
common="$(git rev-parse --path-format=absolute --git-common-dir)"
cat >"$common/hooks/pre-push" <<'SH'
#!/usr/bin/env bash
set -eu
common="$(git rev-parse --path-format=absolute --git-common-dir)"
printf '%s\n' "$@" >"$common/legacy-args"
cat >"$common/legacy-stdin"
printf 'legacy\n' >>"$common/legacy-trace"
SH
chmod +x "$common/hooks/pre-push"
BUILD_WORKSPACE_DIRECTORY="$root" "$installer" "$prek" >/dev/null
failures=0
expect_ok() {
  local name="$1"; shift
  local code=0
  "$@" >"$work/case.log" 2>&1 || code=$?
  if [[ "$code" == 0 ]]; then echo "ok   $name"; else echo "FAIL $name: exit $code"; tail -n 8 "$work/case.log"; failures=$((failures + 1)); fi
}
expect_rejected() {
  local name="$1"; shift
  local code=0
  "$@" >"$work/case.log" 2>&1 || code=$?
  if [[ "$code" != 0 ]]; then echo "ok   $name (exit $code)"; else echo "FAIL $name: accepted"; failures=$((failures + 1)); fi
}
check() { if "$@"; then :; else echo "FAIL invariant: $*"; failures=$((failures + 1)); fi; }
cat >"$work/decoy/prek" <<SH
#!/usr/bin/env bash
printf decoy >"$work/decoy-ran"
exit 0
SH
chmod +x "$work/decoy/prek"
pin="$common/bff-template/bin/prek"
cp "$pin" "$work/saved-pin"
mv "$pin" "$pin.absent"
expect_rejected "absent pin rejects instead of PATH fallback" env PATH="$work/decoy:$PATH" git commit -qm absent-pin --allow-empty
check test ! -e "$work/decoy-ran"
mv "$pin.absent" "$pin"
chmod 644 "$pin"
expect_rejected "non-executable pin rejects instead of PATH fallback" env PATH="$work/decoy:$PATH" git commit -qm nonexec-pin --allow-empty
check test ! -e "$work/decoy-ran"
chmod 755 "$pin"
cat >"$pin" <<SH
#!/usr/bin/env bash
printf tampered >"$work/tampered-ran"
exit 0
SH
chmod +x "$pin"
expect_rejected "tampered executable rejects before invocation" git commit -qm tampered-pin --allow-empty
check test ! -e "$work/tampered-ran"
cp "$work/saved-pin" "$pin"
chmod 755 "$pin"
before="$(sha256sum "$common/hooks/pre-commit" "$common/hooks/pre-push")"
BUILD_WORKSPACE_DIRECTORY="$root" "$installer" "$prek" >/dev/null
check test "$before" = "$(sha256sum "$common/hooks/pre-commit" "$common/hooks/pre-push")"
echo "ok   repeat installation checked"
expect_ok "valid checked-out commit remains usable" git commit -qm valid --allow-empty
expect_ok "valid checked-out single branch push remains usable" git push -q origin main
other="$(printf 'other source\n' | git commit-tree 'HEAD^{tree}' -p HEAD)"
git update-ref refs/heads/other "$other"
git commit -qm next --allow-empty
expect_rejected "two source updates reject before first-ref selection" git push -q origin main other
expect_rejected "different source tip rejects" git push -q origin other
git tag new-tag
expect_rejected "tag creation rejects" git push -q origin refs/tags/new-tag
stage_before="$(wc -l <"$common/stage-trace")"
expect_ok "deletion-only push preserves legacy hook" git push -q origin :delete-only
check test "$stage_before" = "$(wc -l <"$common/stage-trace")"
check test -s "$common/legacy-stdin"
expect_ok "tag deletion introduces no unchecked source" git push -q origin :refs/tags/delete-tag
expect_ok "one current source plus deletion is supported" git push -q origin main :delete-mixed
head="$(git rev-parse HEAD)"
zero="${head//?/0}"
printf 'refs/heads/main %s refs/heads/main %s\n(delete) %s refs/heads/old %s\n' "$head" "$base" "$zero" "$base" >"$work/packet"
stage_before="$(wc -l <"$common/stage-trace")"
expect_ok "original full stdin reaches legacy hook" "$common/hooks/pre-push" origin "$remote" <"$work/packet"
check test "$((stage_before + 1))" = "$(wc -l <"$common/stage-trace")"
check cmp -s "$work/packet" "$common/legacy-stdin"
printf 'origin\n%s\n' "$remote" >"$work/args"
check cmp -s "$work/args" "$common/legacy-args"
# C1: admission must agree with pinned prek's literal-space row parser.
for separator in tabs doubled leading trailing; do
  case "$separator" in
    tabs) printf 'HEAD\t%s\trefs/heads/main\t%s\n' "$head" "$base" ;;
    doubled) printf 'HEAD  %s  refs/heads/main  %s\n' "$head" "$base" ;;
    leading) printf ' HEAD %s refs/heads/main %s\n' "$head" "$base" ;;
    trailing) printf 'HEAD %s refs/heads/main %s \n' "$head" "$base" ;;
  esac >"$work/separator"
  stage_before="$(wc -l <"$common/stage-trace")"
  legacy_before="$(wc -l <"$common/legacy-trace")"
  code=0
  "$common/hooks/pre-push" origin "$remote" <"$work/separator" >"$work/case.log" 2>&1 || code=$?
  if [[ "$code" == 2 ]]; then
    echo "ok   $separator separators rejected by admission"
  else
    echo "FAIL $separator separators: expected admission exit 2, got $code"
    failures=$((failures + 1))
  fi
  check test "$stage_before" = "$(wc -l <"$common/stage-trace")"
  check test "$legacy_before" = "$(wc -l <"$common/legacy-trace")"
done
printf 'malformed\n' >"$work/malformed"
expect_rejected "malformed row rejects instead of being ignored" "$common/hooks/pre-push" origin "$remote" <"$work/malformed"
head -c 1048577 /dev/zero >"$work/oversized"
expect_rejected "oversized original input is rejected" "$common/hooks/pre-push" origin "$remote" <"$work/oversized"
exit "$failures"
