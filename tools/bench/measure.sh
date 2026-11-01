#!/usr/bin/env bash
# Feedback-cost measurement (specs/002-hermetic-skeleton FR-012, T032, T033).
#
#   tools/bench/measure.sh [--cold N] [--warm N] [--weak-isolation] [--stages-only] [--out FILE]
#
# Scenarios, all from a clone of HEAD. The source tree must be clean so the record names exactly
# what was measured.
#   bootstrap:     empty HOME; fetch and verify the pinned toolchain (--bootstrap-only).
#   first_verify:  the full verifier right after bootstrap, which built only the tools; tests are
#                  built and run for the first time.
#   warm_nochange: the full verifier again. Test results are never cached, build outputs are.
#   warm_edited:   the full verifier after a source edit that forces a rebuild and retest.
#   no_download:   --offline: Bazel's --repository_disable_download plus a dead proxy. The
#                  networked provenance check is skipped, so this is not a passing full run, and it
#                  proves only that Bazel and proxy-honoring tools fetched nothing.
#
# --stages-only measures the lifecycle stages instead (spec 006 FR-019), in a clone with the hooks
# installed and a local bare remote. Every commit and push goes through the real hooks:
#   pre_commit_first, pre_push_first: straight after installing in an empty HOME.
#   pre_commit_{1,50,500}:  warm, with that many newly edited Go files staged.
#   pre_commit_rejected:    warm, a staged secret. Each run must be rejected.
#   pre_push_edited:        warm, a real edit to one Go package.
#   pre_push_rejected:      warm, a test in that package made to fail. Each run must be rejected.
#   pre_push_docs_only:     warm, a documentation edit. Bazel's test cache answers, so this is the floor of
#                           the stage and never reported as feedback on edited code.
#
# Peak memory is the highest sampled sum of proportional set size (PSS) over the verifier's process
# tree and the Bazel server's tree, every 0.5 s. PSS divides shared pages among their sharers, so
# they are not double-counted. The sampler is outside both trees. Sampling can miss short spikes.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
cold=3
warm=5
out=""
verify_args=()
isolation=full
stages_only=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --cold) cold="$2"; shift 2 ;;
    --warm) warm="$2"; shift 2 ;;
    --weak-isolation) verify_args=(--weak-isolation); isolation=weakened; shift ;;
    --stages-only) stages_only=true; shift ;;
    --out) out="$2"; shift 2 ;;
    *) echo "measure: unknown argument '$1'" >&2; exit 2 ;;
  esac
done
if [[ -n "$(git -C "$root" status --porcelain)" ]]; then
  echo "measure: the working tree is dirty; commit first so the record names the measured source" >&2
  exit 1
fi
revision="$(git -C "$root" rev-parse HEAD)"
tree="$(git -C "$root" rev-parse 'HEAD^{tree}')"
ncpu="$(nproc)"
mem_gb="$(awk '/MemTotal/ {printf "%d", $2/1024/1024}' /proc/meminfo)"
os="$(awk -F= '/^ID=/{id=$2} /^VERSION_ID=/{v=$2} END{gsub(/"/,"",id); gsub(/"/,"",v); print id "-" v}' /etc/os-release)"
record="$root/docs/budgets/$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)-${ncpu}c"
if $stages_only; then out="${out:-$record-stages.json}"; else out="${out:-$record.json}"; fi
disk_free_gb="$(df -Pk "$root" | awk 'NR==2 {printf "%d", $4/1024/1024}')"
work="$(mktemp -d)"
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT

sample_period=0.5
tree_pss_kb() { # tree_pss_kb <root pid>...: PSS sum over those processes and their descendants
  local pid total=0 kb
  for pid in $(ps -eo pid=,ppid= | awk -v roots="$*" '
    BEGIN { n = split(roots, r, " "); for (i = 1; i <= n; i++) keep[r[i]] = 1 }
    { pid[NR] = $1; parent[$1] = $2 }
    END {
      for (i = 1; i <= NR; i++) {
        p = pid[i]
        while (p > 1) { if (p in keep) { print pid[i]; break } p = parent[p] }
      }
    }'); do
    kb="$(awk '/^Pss:/ {print $2; exit}' "/proc/$pid/smaps_rollup" 2>/dev/null || true)"
    total=$((total + ${kb:-0}))
  done
  echo "$total"
}

timed() { # timed <home> <clone> <env assignments...> -- <command...>; prints "seconds peak_kb rc"
  local home="$1" clone="$2"
  shift 2
  local envs=()
  while [[ "$1" != "--" ]]; do envs+=("$1"); shift; done
  shift
  local start end rc=0 peak=0 now server roots
  start="$(date +%s.%N)"
  mkdir -p "$home/tmp"
  (cd "$clone" && exec env -i HOME="$home" TMPDIR="$home/tmp" PATH=/usr/bin:/bin USER="${USER:-$(id -un)}" TERM=dumb "${envs[@]}" \
    "$@" </dev/null >"$work/run.log" 2>&1) &
  local verifier=$!
  while kill -0 "$verifier" 2>/dev/null; do
    roots="$verifier"
    # The toolchain fixtures start their own server under TMPDIR, which is inside HOME here.
    server="$(find "$home" -maxdepth 7 -name server.pid.txt -exec cat {} + 2>/dev/null | tr '\n' ' ' || true)"
    roots+=" $server"
    now="$(tree_pss_kb "$roots")"
    if ((now > peak)); then peak=$now; fi
    sleep "$sample_period"
  done
  wait "$verifier" || rc=$?
  end="$(date +%s.%N)"
  printf '%s %s %s\n' "$(awk -v a="$start" -v b="$end" 'BEGIN {printf "%.1f", b-a}')" "$peak" "$rc"
}

new_clone() { git clone --quiet --no-local "file://$root" "$1" && git -C "$1" checkout --quiet "$revision"; }
stop_server() { (cd "$2" && env -i HOME="$1" PATH=/usr/bin:/bin tools/bootstrap/bazel.sh shutdown) >/dev/null 2>&1 || true; }

stats() { # stats <file of "seconds peak rc"> -> JSON
  sort -n -k1,1 "$1" | awk '
    { s[NR] = $1; if ($2 > p) p = $2; if ($3 != 0) bad++ }
    END {
      n = NR; i50 = int((n + 1) / 2); i95 = int(n * 0.95 + 0.999)
      if (i95 < 1) i95 = 1; if (i95 > n) i95 = n
      printf "{\"n\": %d, \"p50_s\": %s, \"p95_s\": %s, \"peak_pss_mb\": %d, \"failed_runs\": %d}", n, s[i50], s[i95], p / 1024, bad + 0
    }'
}

# --- The lifecycle stages.
stage_git=(GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null GIT_AUTHOR_NAME=a GIT_AUTHOR_EMAIL=a@example.com
  GIT_COMMITTER_NAME=a GIT_COMMITTER_EMAIL=a@example.com)
stage_package=bff/adapters/memory
stage_file="$stage_package/store.go"
stage_test="$stage_package/store_test.go"

stage_clone() { # stage_clone <base>: a clone on main, a bare remote holding it, and nothing installed
  local base="$1"
  mkdir -p "$base/home/tmp"
  new_clone "$base/clone"
  git -C "$base/clone" checkout --quiet -B main
  git init --quiet --bare -b main "$base/remote.git"
  git -C "$base/clone" remote set-url origin "$base/remote.git"
  git -C "$base/clone" push --quiet origin main
}
in_stage() { # in_stage <base> <command...>: as a hook would see it, outside the timing
  local base="$1"
  shift
  (cd "$base/clone" && env -i HOME="$base/home" TMPDIR="$base/home/tmp" PATH=/usr/bin:/bin USER="${USER:-$(id -un)}" TERM=dumb \
    PREK_HOME="$base/home/prek" "${stage_git[@]}" "$@" </dev/null >>"$work/setup.log" 2>&1)
}
stage_timed() { # stage_timed <base> <command...>
  local base="$1"
  shift
  timed "$base/home" "$base/clone" PREK_HOME="$base/home/prek" "${stage_git[@]}" -- "$@"
}
stage_stop() { # stage_stop <base>: both servers, the push one by its own output base
  local base="$1" output
  in_stage "$base" tools/bootstrap/bazel.sh shutdown || true
  for output in "$base"/home/.cache/bff-template/push/*; do
    if [[ -d "$output" ]]; then in_stage "$base" tools/bootstrap/bazel.sh "--output_base=$output" shutdown || true; fi
  done
}
staged_files() { # staged_files <base> <count> <run>: that many new Go files, unlike any earlier run's
  local base="$1" count="$2" run="$3" i
  mkdir -p "$base/clone/bench_files"
  for ((i = 1; i <= count; i++)); do
    printf 'package bench\n\nconst Value%d = %d\n' "$i" "$run" >"$base/clone/bench_files/f$i.go"
  done
  in_stage "$base" git add bench_files
}
expect_rejected() { # expect_rejected <file of "seconds peak rc">: a rejection that did not happen measures nothing
  if awk '$3 == 0 { found = 1 } END { exit !found }' "$1"; then
    echo "measure: a run that had to be rejected passed; see $1" >&2
    tail -n 20 "$work/run.log" >&2
    exit 1
  fi
}
expect_passed() { # expect_passed <file>: a run that failed for some other reason is not a sample
  if awk '$3 != 0 { found = 1 } END { exit !found }' "$1"; then
    echo "measure: a run that had to pass failed; see $1" >&2
    tail -n 20 "$work/run.log" >&2
    exit 1
  fi
}

stage_scenarios() {
  sample_period=0.1
  local base="$work/stages" i size install=(//tools/hooks:install)
  local bazel_flags=()
  if [[ "$isolation" == weakened ]]; then
    bazel_flags=(--config=weak-isolation)
    install+=(-- --weak-isolation)
  fi
  : >"$work/install"
  : >"$work/commit_first"
  : >"$work/push_first"
  for i in $(seq 1 "$cold"); do
    stage_clone "$base"
    stage_timed "$base" tools/bootstrap/bazel.sh run "${bazel_flags[@]}" "${install[@]}" >>"$work/install"
    staged_files "$base" 1 "$i"
    stage_timed "$base" git commit -q -m first >>"$work/commit_first"
    stage_timed "$base" git push -q origin main >>"$work/push_first"
    echo "cold $i: install $(tail -1 "$work/install"); commit $(tail -1 "$work/commit_first"); push $(tail -1 "$work/push_first")" >&2
    expect_passed "$work/install"
    expect_passed "$work/commit_first"
    expect_passed "$work/push_first"
    if ((i < cold)); then
      stage_stop "$base"
      chmod -R u+w "$base"
      rm -rf "${base:?}"
    fi
  done

  for size in 1 50 500; do
    : >"$work/commit_$size"
    for i in $(seq 1 "$warm"); do
      staged_files "$base" "$size" "$((size * 1000 + i))"
      stage_timed "$base" git commit -q -m warm >>"$work/commit_$size"
      in_stage "$base" git reset -q --hard HEAD~1
    done
    expect_passed "$work/commit_$size"
    echo "pre-commit, $size files: $(stats "$work/commit_$size")" >&2
  done

  : >"$work/commit_rejected"
  mkdir -p "$base/clone/bench_files"
  for i in $(seq 1 "$warm"); do
    printf 'token = "%s%s%03d"\n' "ghp_" "aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0hJ3l" "$i" >"$base/clone/bench_files/config.txt"
    in_stage "$base" git add bench_files
    stage_timed "$base" git commit -q -m rejected >>"$work/commit_rejected"
    in_stage "$base" git reset -q --hard HEAD
  done
  expect_rejected "$work/commit_rejected"
  echo "pre-commit, rejected: $(stats "$work/commit_rejected")" >&2

  : >"$work/push_edited"
  for i in $(seq 1 "$warm"); do
    echo "// Edit $i forces a rebuild and a retest of this package." >>"$base/clone/$stage_file"
    in_stage "$base" git add "$stage_file"
    in_stage "$base" git commit -q --no-verify -m edited
    stage_timed "$base" git push -q origin main >>"$work/push_edited"
    echo "push edited $i: $(tail -1 "$work/push_edited")" >&2
  done
  expect_passed "$work/push_edited"

  : >"$work/push_rejected"
  for i in $(seq 1 "$warm"); do
    printf '\nfunc init() { panic("edit %d fails this package'"'"'s tests") }\n' "$i" >>"$base/clone/$stage_test"
    in_stage "$base" git add "$stage_test"
    in_stage "$base" git commit -q --no-verify -m rejected
    stage_timed "$base" git push -q origin main >>"$work/push_rejected"
    echo "push rejected $i: $(tail -1 "$work/push_rejected")" >&2
    in_stage "$base" git reset -q --hard HEAD~1
  done
  expect_rejected "$work/push_rejected"

  : >"$work/push_docs"
  for i in $(seq 1 "$warm"); do
    echo "<!-- edit $i -->" >>"$base/clone/docs/quality-framework.md"
    in_stage "$base" git add docs/quality-framework.md
    in_stage "$base" git commit -q --no-verify -m docs
    stage_timed "$base" git push -q origin main >>"$work/push_docs"
  done
  expect_passed "$work/push_docs"
  echo "push, docs only: $(stats "$work/push_docs")" >&2
  stage_stop "$base"

  mkdir -p "$(dirname "$out")"
  cat >"$out" <<JSON
{
  "schema_version": 3,
  "revision": "$revision",
  "tree": "$tree",
  "measured_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "isolation": "$isolation",
  "host_profile": {"os": "$os", "kernel": "$(uname -r)", "arch": "$(uname -m)", "cpus": $ncpu, "memory_gb": $mem_gb, "disk_free_gb": $disk_free_gb},
  "provisional_targets": {"pre_commit_warm_p95_s": 5, "pre_push_warm_p95_s": 90},
  "hook_install": $(stats "$work/install"),
  "pre_commit_first": $(stats "$work/commit_first"),
  "pre_push_first": $(stats "$work/push_first"),
  "pre_commit_1": $(stats "$work/commit_1"),
  "pre_commit_50": $(stats "$work/commit_50"),
  "pre_commit_500": $(stats "$work/commit_500"),
  "pre_commit_rejected": $(stats "$work/commit_rejected"),
  "pre_push_edited": $(stats "$work/push_edited"),
  "pre_push_rejected": $(stats "$work/push_rejected"),
  "pre_push_docs_only": $(stats "$work/push_docs"),
  "limits": "Times are a whole git commit or git push through the real hooks, including prek. failed_runs equals n on the rejected scenarios by construction. pre_commit_N staged N new Go files, so it exercises the content checks that apply to Go and nothing for other languages. pre_push_docs_only is answered by Bazel's test cache and is the stage's floor, not feedback on edited code. pre_push_edited rebuilds one package and retests what depends on it. Install and first runs include downloads when the host's download cache is empty, so they depend on the link. Peak memory is sampled PSS of the command's and Bazel servers' process trees every 0.1 s and can miss short spikes."
}
JSON
  echo "measure: wrote $out" >&2
  cat "$out"
}

if $stages_only; then
  stage_scenarios
  exit 0
fi

: >"$work/bootstrap"
: >"$work/first"
for i in $(seq 1 "$cold"); do
  mkdir -p "$work/cold$i/home"
  new_clone "$work/cold$i/clone"
  timed "$work/cold$i/home" "$work/cold$i/clone" -- tools/verify.sh "${verify_args[@]}" --bootstrap-only >>"$work/bootstrap"
  timed "$work/cold$i/home" "$work/cold$i/clone" -- tools/verify.sh "${verify_args[@]}" >>"$work/first"
  stop_server "$work/cold$i/home" "$work/cold$i/clone"
  echo "cold $i: bootstrap $(tail -1 "$work/bootstrap"); first verify $(tail -1 "$work/first")" >&2
  chmod -R u+w "$work/cold$i"
  rm -rf "$work/cold$i"
done

mkdir -p "$work/warm/home"
new_clone "$work/warm/clone"
timed "$work/warm/home" "$work/warm/clone" -- tools/verify.sh "${verify_args[@]}" >/dev/null
: >"$work/nochange"
: >"$work/edited"
for i in $(seq 1 "$warm"); do
  timed "$work/warm/home" "$work/warm/clone" -- tools/verify.sh "${verify_args[@]}" >>"$work/nochange"
  echo "warm unchanged $i: $(tail -1 "$work/nochange")" >&2
done
for i in $(seq 1 "$warm"); do
  echo "// Edit $i forces a rebuild and a retest of this package." >>"$work/warm/clone/quality/checks/suppression/scan.go"
  timed "$work/warm/home" "$work/warm/clone" -- tools/verify.sh "${verify_args[@]}" >>"$work/edited"
  echo "warm edited $i: $(tail -1 "$work/edited")" >&2
done
git -C "$work/warm/clone" checkout --quiet -- .
timed "$work/warm/home" "$work/warm/clone" HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 \
  -- tools/verify.sh "${verify_args[@]}" --offline >"$work/nodownload"
echo "no download: $(cat "$work/nodownload")" >&2
stop_server "$work/warm/home" "$work/warm/clone"

mkdir -p "$(dirname "$out")"
cat >"$out" <<JSON
{
  "schema_version": 2,
  "revision": "$revision",
  "tree": "$tree",
  "measured_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "isolation": "$isolation",
  "host_profile": {"os": "$os", "kernel": "$(uname -r)", "arch": "$(uname -m)", "cpus": $ncpu, "memory_gb": $mem_gb, "disk_free_gb": $disk_free_gb},
  "bootstrap": $(stats "$work/bootstrap"),
  "first_verify": $(stats "$work/first"),
  "warm_nochange": $(stats "$work/nochange"),
  "warm_edited": $(stats "$work/edited"),
  "no_download": $(stats "$work/nodownload"),
  "limits": "bootstrap includes downloads, so its time depends on the link. warm_nochange reruns every test but reuses build outputs; warm_edited rebuilds one Go package and its dependents. no_download skips networked provenance and proves only that Bazel fetched nothing and proxy-honoring tools reached no network; it is not a network-isolation claim. Peak memory is sampled PSS of the verifier and Bazel server process trees every 0.5 s and can miss short spikes."
}
JSON
echo "measure: wrote $out" >&2
cat "$out"
