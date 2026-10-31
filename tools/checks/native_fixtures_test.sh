#!/usr/bin/env bash
# Defect, corrected and tamper fixtures for the native checks (specs/002-hermetic-skeleton T006,
# T026, T027, T034). Cases run through tools/checks/native.sh, the verifier's own invocation, and
# a failure only counts when it names the intended reason. Defects are generated here so the
# repository's own scans never see them.
set -euo pipefail
for pair in "$@"; do export "${pair%%=*}=$(realpath "${pair#*=}")"; done
native="$PWD/tools/checks/native.sh"
repo_gitleaks_config="$PWD/.gitleaks.toml"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
export HOME="$work/home"
mkdir -p "$HOME"
failures=0
cases=0
dir="$work" # the working directory of the next cases

expect() { # expect <pass|fail> <diagnostic substring or -> <label> <command...>
  local want="$1" needle="$2" label="$3" rc=0
  shift 3
  cases=$((cases + 1))
  (cd "$dir" && "$@") </dev/null >"$work/out" 2>&1 || rc=$?
  local got=pass
  [[ $rc -eq 0 ]] || got=fail
  if [[ "$got" != "$want" ]]; then
    echo "FAIL $label: $got, want $want" >&2
    sed 's/^/  /' "$work/out" | head -20 >&2
    failures=$((failures + 1))
  elif [[ "$needle" != "-" ]] && ! grep -qF -- "$needle" "$work/out"; then
    echo "FAIL $label: failed, but not with '$needle'" >&2
    sed 's/^/  /' "$work/out" | head -20 >&2
    failures=$((failures + 1))
  else
    echo "ok   $label"
  fi
}

list() { # list <dir> <paths...>: writes <dir>.list (NUL) and <dir>.nl (newline)
  local dir="$1"
  shift
  printf '%s\0' "$@" >"$dir.list"
  printf '%s\n' "$@" >"$dir.nl"
}
token() { printf '%s%s' "ghp_" "aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0hJ3lN6"; }

# --- secrets (T034, plus .gitleaksignore found while reproducing T026)
s="$work/s" && mkdir -p "$s/bazel-config"
printf 'token = "%s"\n' "$(token)" >"$s/config.txt"
printf 'token = read_from_environment()\n' >"$s/clean.txt"
printf 'token = "%s"\n' "$(token)" >"$s/bazel-config/x.txt"
printf 'token = "%s" # gitleaks:%s\n' "$(token)" "allow" >"$s/inline.txt"
dir="$s"
list "$work/s-bad" config.txt
expect fail "leaks found" "secrets defect" "$native" secrets "$work/s-bad.list" "$repo_gitleaks_config"
list "$work/s-good" clean.txt
expect pass - "secrets corrected" "$native" secrets "$work/s-good.list" "$repo_gitleaks_config"
list "$work/s-prefixed" bazel-config/x.txt
expect fail "leaks found" "secrets: a tracked bazel-* lookalike directory is scanned" "$native" secrets "$work/s-prefixed.list" "$repo_gitleaks_config"
list "$work/s-inline" inline.txt
expect fail "leaks found" "secrets: inline allow comment is ignored" "$native" secrets "$work/s-inline.list" "$repo_gitleaks_config"
"$GITLEAKS" dir --no-banner --no-color --exit-code 0 --report-format json --report-path "$work/report.json" "$s" </dev/null >/dev/null 2>&1
grep -o '"Fingerprint": "[^"]*"' "$work/report.json" | cut -d'"' -f4 >"$s/.gitleaksignore"
[[ -s "$s/.gitleaksignore" ]] || { echo "FAIL could not build .gitleaksignore fixture" >&2; failures=$((failures + 1)); }
list "$work/s-ignored" config.txt .gitleaksignore
expect fail "leaks found" "secrets: .gitleaksignore cannot hide a finding" "$native" secrets "$work/s-ignored.list" "$repo_gitleaks_config"
: >"$work/none.list"
expect fail "empty input list" "secrets: an empty list is not a pass" "$native" secrets "$work/none.list" "$repo_gitleaks_config"

# --- typos (T026)
t="$work/t" && mkdir -p "$t/sub"
printf '[files]\n' >"$t/canonical.toml"
printf 'The envir%s is set.\n' "onemnt" >"$t/sub/bad.txt"
printf 'The environment is set.\n' >"$t/sub/good.txt"
dir="$t"
list "$work/t-bad" sub/bad.txt
expect fail "error:" "typos defect" "$native" typos "$work/t-bad.nl" canonical.toml
list "$work/t-good" sub/good.txt
expect pass - "typos corrected" "$native" typos "$work/t-good.nl" canonical.toml
printf '[default]\ncheck-file = false\n' >"$t/sub/.typos.toml"
expect fail "error:" "typos: nested config cannot disable the check" "$native" typos "$work/t-bad.nl" canonical.toml
: >"$work/none.nl"
expect fail "empty input list" "typos: an empty list is not a pass" "$native" typos "$work/none.nl" canonical.toml

# --- shellcheck (T026)
c="$work/c/nested" && mkdir -p "$c"
cat >"$c/bad.sh" <<'SH'
#!/bin/sh
echo $1
SH
cat >"$c/good.sh" <<'SH'
#!/bin/sh
echo "$1"
SH
dir="$c"
list "$work/c-bad" bad.sh
expect fail "SC2086" "shellcheck defect" "$native" shellcheck "$work/c-bad.list"
list "$work/c-good" good.sh
expect pass - "shellcheck corrected" "$native" shellcheck "$work/c-good.list"
echo 'disable=all' >"$c/.shellcheckrc"
expect fail "SC2086" "shellcheck: same-directory rc cannot disable" "$native" shellcheck "$work/c-bad.list"
rm "$c/.shellcheckrc"
echo 'disable=all' >"$work/c/.shellcheckrc"
expect fail "SC2086" "shellcheck: parent rc cannot disable" "$native" shellcheck "$work/c-bad.list"
rm "$work/c/.shellcheckrc"
echo 'disable=all' >"$HOME/.shellcheckrc"
expect fail "SC2086" "shellcheck: home rc cannot disable" "$native" shellcheck "$work/c-bad.list"
rm "$HOME/.shellcheckrc"
expect fail "SC2086" "shellcheck: SHELLCHECK_OPTS cannot disable" env SHELLCHECK_OPTS=--exclude=SC2086 "$native" shellcheck "$work/c-bad.list"
expect fail "empty input list" "shellcheck: an empty list is not a pass" "$native" shellcheck "$work/none.list"

# --- buildifier (T026)
b="$work/b" && mkdir -p "$b"
printf 'glob(["a"])\n' >"$b/BUILD.bazel"
dir="$b"
list "$work/b-bad" BUILD.bazel
expect fail "constant-glob" "buildifier defect" "$native" buildifier "$work/b-bad.list"
printf '{"warningsList": ["-constant-glob"]}\n' >"$b/.buildifier.json"
expect fail "constant-glob" "buildifier: .buildifier.json cannot disable" "$native" buildifier "$work/b-bad.list"
expect pass - "buildifier: the config file really would disable it (control)" "$BUILDIFIER" --mode=check --lint=warn BUILD.bazel
rm "$b/.buildifier.json"
printf 'exports_files(["a"])\n' >"$b/BUILD.bazel"
expect pass - "buildifier corrected" "$native" buildifier "$work/b-bad.list"

# --- actionlint (T027): verdicts cannot depend on what is on PATH
a="$work/a" && mkdir -p "$a/.github/workflows" "$work/stub"
cat >"$a/.github/workflows/bad.yml" <<'YAML'
on: push
jobs:
  a:
    runs-on: ubuntu-24.04
    steps:
      - run: echo $FOO
YAML
cat >"$a/.github/workflows/good.yml" <<'YAML'
on: push
jobs:
  a:
    runs-on: ubuntu-24.04
    steps:
      - run: echo "$FOO"
      - run: print("hello")
        shell: python
YAML
printf '#!/bin/sh\nexit 0\n' >"$work/stub/shellcheck"
printf '#!/bin/sh\necho "stub:1:1: undeclared helper ran"\nexit 1\n' >"$work/stub/pyflakes"
chmod +x "$work/stub/shellcheck" "$work/stub/pyflakes"
git -C "$a" init -q
dir="$a"
list "$work/a-bad" .github/workflows/bad.yml
list "$work/a-good" .github/workflows/good.yml
expect fail "SC2086" "actionlint defect, no host helpers" env PATH=/usr/bin:/bin "$native" actionlint "$work/a-bad.list"
expect fail "SC2086" "actionlint: a no-op host shellcheck cannot hide it" env PATH="$work/stub:/usr/bin:/bin" "$native" actionlint "$work/a-bad.list"
expect pass - "actionlint corrected, no host helpers" env PATH=/usr/bin:/bin "$native" actionlint "$work/a-good.list"
expect pass - "actionlint: an undeclared host pyflakes cannot fail it" env PATH="$work/stub:/usr/bin:/bin" "$native" actionlint "$work/a-good.list"

# --- zizmor (T026)
z="$work/z" && mkdir -p "$z/.github/workflows"
cat >"$z/.github/workflows/w.yml" <<'YAML'
name: w
on: push
permissions: {}
jobs:
  a:
    name: a
    runs-on: ubuntu-24.04
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
YAML
git -C "$z" init -q
dir="$z"
list "$work/z-all" .github/workflows/w.yml
expect fail "artipacked" "zizmor defect" "$native" zizmor "$work/z-all.list"
printf 'rules:\n  artipacked:\n    disable: true\n' >"$z/.github/zizmor.yml"
expect fail "artipacked" "zizmor: repository config cannot disable" "$native" zizmor "$work/z-all.list"
rm "$z/.github/zizmor.yml"
sed -i 's|      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1|&\n        with:\n          persist-credentials: false|' "$z/.github/workflows/w.yml"
sed -i 's|^permissions: {}|&\nconcurrency:\n  group: w-${{ github.sha }}\n  cancel-in-progress: false|' "$z/.github/workflows/w.yml"
expect pass - "zizmor corrected" "$native" zizmor "$work/z-all.list"

# --- suppression-ban binary (T029)
u="$work/u" && mkdir -p "$u"
printf 'package x\n\nfunc f() {} //%s:unused\n' "nolint" >"$u/bad.go"
printf 'package x\n\nfunc f() {}\n' >"$u/good.go"
printf 'bad.go\0' >"$work/u-bad.list"
printf 'good.go\0' >"$work/u-good.list"
expect fail "[S001]" "suppression-ban defect" "$SUPPRESSION" -root "$u" -paths "$work/u-bad.list"
expect pass - "suppression-ban corrected" "$SUPPRESSION" -root "$u" -paths "$work/u-good.list"
expect fail "path list is empty" "suppression-ban: an empty list is not a pass" "$SUPPRESSION" -root "$u" -paths "$work/none.list"
: >"$work/empty.manifest"
expect fail "malformed manifest" "suppression-ban: an empty manifest fails closed" "$SUPPRESSION" -root "$u" -paths "$work/u-good.list" -exempt "$work/empty.manifest"

echo "cases=$cases failures=$failures"
[[ $cases -ge 30 ]] || { echo "FAIL expected at least 30 cases, ran $cases" >&2; exit 1; }
exit "$failures"
