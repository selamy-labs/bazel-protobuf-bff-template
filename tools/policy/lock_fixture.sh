#!/usr/bin/env bash
# Toolchain witnesses that need a real Bazel (specs/002-hermetic-skeleton US1, T030, T032):
#   1. an unlocked MODULE.bazel edit fails under the repository's .bazelrc, and the unmodified copy
#      passes;
#   2. a native tool whose checksum does not match its pin is rejected, not used;
#   3. a hostile HOME rc breaks raw Bazel (control) but cannot reach the pinned wrapper;
#   4. every target analyzes with no C compiler on PATH, and a real C consumer fails even when the
#      host has a compiler (specs/005-browser-client FR-012);
#   5. a vet or staticcheck finding fails the Go build, in the root and in the quality module, and the
#      corrected code builds (specs/006-quality-hooks FR-016).
# Runs in a throwaway copy with its own output root, sharing only the download cache. The fourth
# reads the real checkout, with its own output root and no convenience links, because it must see
# every target.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
bazel="$root/tools/bootstrap/bazel.sh"
work="$(mktemp -d)"
trap '(cd "$work/ws" 2>/dev/null && "$bazel" --output_user_root="$work/out" shutdown) >/dev/null 2>&1; (cd "$root" && "$bazel" --output_user_root="$work/out3" shutdown) >/dev/null 2>&1; chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT
trap 'exit 143' TERM INT # An EXIT trap does not run when bash dies of a signal.
cache="$(cd "$root" && "$bazel" info repository_cache)"

mkdir -p "$work/ws"
cp "$root"/{.bazelversion,.bazelrc,MODULE.bazel,MODULE.bazel.lock,REPO.bazel,go.mod,go.sum} "$work/ws/"
cp "$root"/{package.json,pnpm-lock.yaml,pnpm-workspace.yaml} "$work/ws/"
cp -r "$root/quality" "$work/ws/quality"
mkdir -p "$work/ws/tools"
cp -r "$root/tools/browser" "$work/ws/tools/browser"
touch "$work/ws/BUILD.bazel"

in_ws() { (cd "$work/ws" && "$bazel" --output_user_root="$work/out" "$@" --repository_cache="$cache" </dev/null >"$work/log" 2>&1); }

in_ws mod deps || { echo "lock-fixture: unmodified copy failed:" >&2; tail -5 "$work/log" >&2; exit 1; }
cp "$work/ws/MODULE.bazel" "$work/MODULE.clean"
# Bazel 9 locks registry file hashes and extension results, not the resolved graph, so the witness
# adds a module version whose registry files the lock has never recorded.
echo 'bazel_dep(name = "bazel_skylib", version = "1.9.2")' >>"$work/ws/MODULE.bazel"
if in_ws mod deps; then
  echo "lock-fixture: unlocked MODULE.bazel edit was accepted" >&2
  exit 1
fi
grep -qi 'lockfile' "$work/log" || { echo "lock-fixture: failure did not name the lockfile:" >&2; tail -5 "$work/log" >&2; exit 1; }
cp "$work/MODULE.clean" "$work/ws/MODULE.bazel"
echo "ok   lock-fixture defect and corrected"

printf 'not the pinned tool\n' >"$work/tampered.tar.gz"
wrong="$(printf 'something else' | sha256sum | cut -d' ' -f1)"
"$YQ" -i -o=json "(.shellcheck.binaries[]) |= (.url = \"file://$work/tampered.tar.gz\" | .sha256 = \"$wrong\")" "$work/ws/quality/multitool.lock.json"
# update mode keeps the lock's registry hashes, so this needs no registry access; the tampered tool
# is fetched from file://, and only its checksum can fail.
if in_ws build --config=weak-isolation --nobuild --lockfile_mode=update @bff_template_quality//tools:shellcheck; then
  echo "checksum-fixture: a tool with the wrong checksum was accepted" >&2
  exit 1
fi
grep -qi 'checksum' "$work/log" || { echo "checksum-fixture: failure did not name the checksum:" >&2; tail -5 "$work/log" >&2; exit 1; }
echo "ok   checksum-fixture defect"

mkdir -p "$work/hostile"
echo 'common --option_that_only_a_hostile_rc_would_set' >"$work/hostile/.bazelrc"
binary="$("$bazel" --path)"
cached="${XDG_CACHE_HOME:-$HOME/.cache}"
if (cd "$work/ws" && HOME="$work/hostile" "$binary" --batch --output_user_root="$work/out2" version </dev/null >"$work/log" 2>&1); then
  echo "ambient-fixture: control failed; raw Bazel ignored the hostile rc, so this proves nothing" >&2
  exit 1
fi
grep -qF 'option_that_only_a_hostile_rc_would_set' "$work/log" || { echo "ambient-fixture: control failed for another reason:" >&2; tail -3 "$work/log" >&2; exit 1; }
for hostile_env in "HOME=$work/hostile" "BAZELRC=$work/hostile/.bazelrc"; do
  (cd "$work/ws" && env "XDG_CACHE_HOME=$cached" "$hostile_env" "$bazel" --batch --output_user_root="$work/out2" version </dev/null >"$work/log" 2>&1) ||
    { echo "ambient-fixture: $hostile_env reached the pinned wrapper:" >&2; tail -3 "$work/log" >&2; exit 1; }
done
echo "ok   ambient-fixture control and corrected"

# The compilers Bazel's C++ detection and rules_go could reach through PATH.
mkdir -p "$work/no-compiler"
for tool in /usr/bin/* /bin/*; do
  name="${tool##*/}"
  case "$name" in
    cc | c++ | c89* | c99* | cpp | cpp-* | gcc | gcc-* | g++ | g++-* | clang | clang++ | clang-* | tcc) continue ;;
    *-gcc | *-gcc-* | *-g++ | *-g++-* | *-cpp | *-cpp-*) continue ;;
  esac
  [[ -e "$work/no-compiler/$name" ]] || ln -s "$tool" "$work/no-compiler/$name"
done
no_compiler() {
  (cd "$root" && env -u CC PATH="$work/no-compiler" "$bazel" --output_user_root="$work/out3" build --config=weak-isolation \
    --nobuild --symlink_prefix=/ "$@" --repository_cache="$cache" </dev/null >"$work/log" 2>&1)
}
if no_compiler --repo_env=BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=0 @bff_template_quality//checks/suppression:suppression-ban; then
  echo "compiler-fixture: control failed; analysis passed with detection on, so this proves nothing" >&2
  exit 1
fi
grep -qF 'Cannot find gcc or CC' "$work/log" || { echo "compiler-fixture: control failed for another reason:" >&2; tail -5 "$work/log" >&2; exit 1; }
no_compiler //... @bff_template_quality//... || { echo "compiler-fixture: a target needs a host C compiler:" >&2; tail -8 "$work/log" >&2; exit 1; }

mkdir -p "$work/ws/c_consumer"
cat >"$work/ws/c_consumer/BUILD.bazel" <<'FIXTURE'
genrule(
    name = "main",
    outs = ["main.txt"],
    cmd = "echo $(CC) >$@",
    toolchains = ["@bazel_tools//tools/cpp:current_cc_toolchain"],
)
FIXTURE
if in_ws build --config=weak-isolation --nobuild //c_consumer:main; then
  echo "compiler-fixture: a C consumer used the host compiler" >&2
  exit 1
fi
grep -qF 'Unable to find a CC toolchain' "$work/log" || { echo "compiler-fixture: failure did not name the C toolchain:" >&2; tail -8 "$work/log" >&2; exit 1; }
echo "ok   compiler-fixture control and corrected"

# package <directory>: one library that the analyzers judge when it builds.
go_package() {
  mkdir -p "$1"
  cat >"$1/BUILD.bazel" <<'FIXTURE'
load("@rules_go//go:def.bzl", "go_library")

go_library(
    name = "fixture",
    srcs = ["fixture.go"],
    importpath = "example.com/fixture",
    visibility = ["//visibility:public"],
)
FIXTURE
}
analyzed() { # analyzed <directory> <target> <name> <finding> <defect body>: rejected, then corrected
  local dir="$1" target="$2" name="$3" finding="$4" body="$5"
  printf 'package fixture\n\nimport "fmt"\n\nfunc Value() string { return fmt.Sprint(1) }\n' >"$dir/fixture.go"
  in_ws build --config=weak-isolation "$target" || { echo "analysis-fixture: $name: valid code failed:" >&2; tail -8 "$work/log" >&2; exit 1; }
  printf 'package fixture\n\nimport "fmt"\n\nfunc Value() string {\n%s\n\treturn fmt.Sprint(1)\n}\n' "$body" >"$dir/fixture.go"
  if in_ws build --config=weak-isolation "$target"; then
    echo "analysis-fixture: $name: the build accepted the finding" >&2
    exit 1
  fi
  grep -qF "$finding" "$work/log" || { echo "analysis-fixture: $name: failed without '$finding':" >&2; tail -8 "$work/log" >&2; exit 1; }
}
go_package "$work/ws/analysis_fixture"
go_package "$work/ws/quality/analysis_fixture"
analyzed "$work/ws/analysis_fixture" //analysis_fixture:fixture root-vet '(printf)' $'\t_ = fmt.Sprintf("%d", "text")'
analyzed "$work/ws/analysis_fixture" //analysis_fixture:fixture root-staticcheck '(SA5000)' $'\tvar counts map[string]int\n\tcounts["a"] = 1'
analyzed "$work/ws/quality/analysis_fixture" @bff_template_quality//analysis_fixture:fixture quality-vet '(printf)' $'\t_ = fmt.Sprintf("%d", "text")'
echo "ok   analysis-fixture defect and corrected"
