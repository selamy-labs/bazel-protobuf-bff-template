#!/usr/bin/env bash
# Feedback-cost measurement (specs/002-hermetic-skeleton FR-012, T032, T033).
#
#   tools/bench/measure.sh --base=<rev> [--cold N] [--warm N] [--weak-isolation] [--stages-only]
#                          [--edited-only] [--evidence DIR] [--out FILE]
#
# Scenarios, all from a clone of HEAD. The source tree must be clean so the record names exactly
# what was measured.
#   bootstrap:     empty HOME; fetch and verify the pinned toolchain (--bootstrap-only).
#   first_verify:  the full verifier right after bootstrap, which built only the tools; tests are
#                  built and run for the first time.
#   warm_nochange: the full verifier again. Test results are never cached, build outputs are.
#   warm_edited:   the full verifier after a source edit that forces a rebuild and retest.
#   no_download:   --offline: Bazel's --repository_disable_download plus a dead proxy. The
#                  networked provenance check is skipped and building the base's evaluator is refused,
#                  so this is never a passing full run. It is recorded apart from the samples, with its
#                  exit code, and proves only that Bazel and proxy-honoring tools fetched nothing.
#
# A scenario that has to pass and did not is refused: the harness stops and writes no record, because
# the time of a failed run is not a measurement of that scenario.
#
# Every timed run's log and receipts are copied out of the clone before the next run can replace them,
# into DIR/run-NNNN, with DIR/index.tsv naming each. --evidence defaults to .verify/ in this checkout.
#
# On any exit this run's private servers are stopped before its work tree is deleted. If that cannot be
# shown, the tree is kept, DIR/unreconciled.txt says why, and a run that had passed exits 3.
#
# --edited-only runs the warm clone's first full verifier, then only warm_edited. It needs --out, so
# that a partial record cannot replace a whole one.
#
# --stages-only measures the lifecycle stages instead (spec 006 FR-019), in a clone with the hooks
# installed and a local bare remote. Every commit and push goes through the real hooks:
#   pre_commit_first, pre_push_first: straight after installing in an empty HOME.
#   pre_commit_{1,50,500}:  warm, with that many newly edited Go files staged.
#   pre_commit_rejected:    warm, a staged secret. Each run must be rejected.
#   pre_push_edited:        warm, a comment appended to one Go package.
#   pre_push_exported:      warm, a new exported constant appended to that package.
#   pre_push_runtime:       warm, a new init function in that package, which the linker keeps in every
#                           importer, so this changes what runs. Its test result must update each time.
#   pre_push_rejected:      warm, a test in that package made to fail. Each run must be rejected, for
#                           that test.
#   pre_push_docs_only:     warm, a documentation edit. Bazel's test cache answers, so this is the floor of
#                           the stage and never reported as feedback on edited code.
# These names say what was edited, not what that caused. Beside its times, each push records spawned
# actions from the receipt and the test.xml files whose path or modification time it changed in the
# private output base (the tests_run fields). That is a proxy, not a cache-aware record of execution, and
# a rewritten result alone does not prove a fresh run. Caching stays on, as in any ordinary push. An
# edited sample that spawned nothing is refused as a cache hit. A rejected sample must update the
# package's own result, which is the control for that probe.
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
policy_base=""
isolation=full
stages_only=false
edited_only=false
evidence=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --cold) cold="$2"; shift 2 ;;
    --warm) warm="$2"; shift 2 ;;
    --weak-isolation) verify_args+=(--weak-isolation); isolation=weakened; shift ;;
    --base=*) verify_args+=("$1"); policy_base="${1#--base=}"; shift ;;
    --stages-only) stages_only=true; shift ;;
    --edited-only) edited_only=true; shift ;;
    --evidence) evidence="$2"; shift 2 ;;
    --out) out="$2"; shift 2 ;;
    *) echo "measure: unknown argument '$1'" >&2; exit 2 ;;
  esac
done
if $edited_only && { $stages_only || [[ -z "$out" ]]; }; then
  echo "measure: --edited-only needs --out and excludes --stages-only" >&2
  exit 2
fi
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
evidence="${evidence:-$root/.verify/measure-$revision}"
if [[ -e "$evidence" ]]; then
  echo "measure: $evidence already exists; a record names one run's evidence" >&2
  exit 2
fi
mkdir -p "$evidence"
runs=0
work="$(readlink -f "$(mktemp -d)")"

# Every server of this run has its output base under $work. One is taken as ours only while its process
# carries the exact --output_base argument of its pid file's own directory, so a reused PID and another
# producer's server, however alike, are left alone (spec 006 R7). A client is never started here, so a
# server that never ran costs nothing.
live_servers() { # live_servers <file of pid files>: the PIDs of the ones still running
  local file pid
  while IFS= read -r file; do
    pid="$(cat "$file" 2>/dev/null || true)"
    if [[ "$pid" =~ ^[0-9]+$ ]] &&
      { tr '\0' '\n' <"/proc/$pid/cmdline"; } 2>/dev/null | grep -qxF -- "--output_base=${file%/server/server.pid.txt}"; then
      echo "$pid"
    fi
  done <"$1"
}
work_processes() { # any process still running in or naming $work, other than this script
  local dir
  for dir in /proc/[0-9]*; do
    if [[ "${dir#/proc/}" != "$$" ]] && { [[ "$(readlink "$dir/cwd" 2>/dev/null || true)" == "$work"* ]] ||
      [[ "$({ tr '\0' ' ' <"$dir/cmdline"; } 2>/dev/null || true)" == *"$work"* ]]; }; then
      echo "${dir#/proc/}"
    fi
  done
}
stop_private_servers() { # stops them, at most 20 s; fails if they cannot be shown to be gone
  local files signal="" attempt
  local -a pids=()
  files="$(mktemp)"
  if ! timeout 60 find "$work" \( -name execroot -o -name external \) -prune -o -name server.pid.txt -print >"$files"; then
    rm -f "$files"
    return 1
  fi
  for attempt in $(seq 1 100); do
    mapfile -t pids < <(live_servers "$files")
    if ((${#pids[@]} == 0)); then break; fi
    if ((attempt == 1)); then signal=TERM; elif ((attempt == 75)); then signal=KILL; else signal=""; fi
    if [[ -n "$signal" ]]; then kill "-$signal" "${pids[@]}" 2>/dev/null || true; fi
    sleep 0.2
  done
  rm -f "$files"
  ((${#pids[@]} == 0))
}
cleanup() { # the work tree goes only once nothing of this run is left using it
  local rc=$? left="" attempt
  set +e
  stop_private_servers || left="a private server could not be shown to be gone"
  for attempt in 1 2 3 4 5; do
    if [[ -z "$(work_processes)" ]]; then break; fi
    sleep 1
  done
  left+="$(work_processes | tr '\n' ' ')"
  if [[ -n "$left" ]]; then
    echo "measure: not quiescent, so $work is kept for reconciliation; still using it: $left" | tee -a "$evidence/unreconciled.txt" >&2
    if ((rc == 0)); then rc=3; fi
  else
    chmod -R u+w "$work" 2>/dev/null
    rm -rf "$work"
  fi
  exit "$rc"
}
trap cleanup EXIT

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

keep_run() { # keep_run <clone> <sample> <command...>: this run's log and receipts, outside the clone
  local clone="$1" sample="$2" dir
  shift 2
  runs=$((runs + 1))
  dir="$evidence/$(printf 'run-%04d' "$runs")"
  mkdir "$dir"
  cp "$work/run.log" "$dir/run.log"
  if [[ -d "$clone/.verify" ]]; then cp -a "$clone/.verify" "$dir/verify"; fi
  if [[ -d "$clone/.git/bff-template/receipts" ]]; then cp -a "$clone/.git/bff-template/receipts" "$dir/receipts"; fi
  printf '%s\t%s\t%s\n' "${dir##*/}" "$sample" "$*" >>"$evidence/index.tsv"
}

timed() { # timed <home> <clone> <env assignments...> -- <command...>; prints "seconds peak_kb rc"
  local home="$1" clone="$2"
  shift 2
  local envs=()
  while [[ "$1" != "--" ]]; do envs+=("$1"); shift; done
  shift
  local start end rc=0 peak=0 now server roots file sample
  start="$(date +%s.%N)"
  mkdir -p "$home/tmp"
  (cd "$clone" && exec env -i HOME="$home" TMPDIR="$home/tmp" PATH=/usr/bin:/bin USER="${USER:-$(id -un)}" TERM=dumb "${envs[@]}" \
    "$@" </dev/null >"$work/run.log" 2>&1) &
  local verifier=$!
  while kill -0 "$verifier" 2>/dev/null; do
    roots="$verifier"
    if $stages_only; then
      # A search of this tree takes longer than a whole warm stage, so only the known places are read.
      server=""
      for file in "$home"/.cache/bazel/_bazel_*/*/server/server.pid.txt "$home"/.cache/bff-template/push/*/server/server.pid.txt; do
        if [[ -f "$file" ]]; then server+=" $(<"$file")"; fi
      done
    else
      # The toolchain fixtures start their own server under TMPDIR, which is inside HOME here.
      # These files have no final newline, so each is read alone: joined, two PIDs make a wrong third.
      server="$(find "$home" -maxdepth 7 -name server.pid.txt -exec sh -c 'for file; do cat "$file"; echo; done' _ {} + 2>/dev/null | tr '\n' ' ' || true)"
    fi
    roots+=" $server"
    now="$(tree_pss_kb "$roots")"
    if ((now > peak)); then peak=$now; fi
    sleep "$sample_period"
  done
  wait "$verifier" || rc=$?
  end="$(date +%s.%N)"
  sample="$(awk -v a="$start" -v b="$end" 'BEGIN {printf "%.1f", b-a}') $peak $rc"
  keep_run "$clone" "$sample" "$@"
  echo "$sample"
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
    echo "measure: a run that had to pass failed, so no record is written; see $evidence" >&2
    tail -n 20 "$work/run.log" >&2
    exit 1
  fi
}

spawned() { # spawned <base>: the actions the last push spawned, from its receipt
  awk -F'"' '/"sandbox_summary"/ { n = 0; if (match($4, /[0-9]+ (processwrapper|linux)-sandbox/)) n = substr($4, RSTART, RLENGTH) + 0; print n }' \
    "$1/clone/.git/bff-template/receipts/pre-push.json"
}
test_logs() { # test_logs <base>: each test's result file and when a run last wrote it
  { find "$1"/home/.cache/bff-template/push/*/execroot/_main/bazel-out/*/testlogs -name test.xml -printf '%P %T@\n' 2>/dev/null || true; } | sort
}
observed_push() { # observed_push <base> <label>: one timed push, with the work it really did
  local base="$1" label="$2" total own actions
  test_logs "$base" >"$work/logs_before"
  stage_timed "$base" git push -q origin main >>"$work/push_$label"
  test_logs "$base" >"$work/logs_after"
  comm -13 "$work/logs_before" "$work/logs_after" >"$work/logs_new"
  total="$(wc -l <"$work/logs_new")"
  own="$(grep -c "^$stage_package/" "$work/logs_new" || true)"
  actions="$(spawned "$base")"
  echo "${actions:-0}" >>"$work/spawns_$label"
  echo "$total" >>"$work/tests_$label"
  echo "$own" >>"$work/own_$label"
  echo "$((total - own))" >>"$work/dependents_$label"
}
expect_work() { # expect_work <label>: an edited sample that spawned nothing is a cache hit, not feedback
  if [[ "$(tail -1 "$work/spawns_$1")" -eq 0 ]]; then
    echo "measure: $1: the receipt shows no action ran, so this is not an edited sample" >&2
    exit 1
  fi
}
range() { sort -n "$1" | awk 'NR == 1 { low = $1 } { high = $1 } END { printf "{\"min\": %d, \"max\": %d}", low, high }'; }
work_record() { # work_record <label>: what the samples of one scenario really did
  printf '{"spawned_actions": %s, "tests_run": %s, "dependent_tests_run": %s, "samples_running_the_package_test": %d}' \
    "$(range "$work/spawns_$1")" "$(range "$work/tests_$1")" "$(range "$work/dependents_$1")" \
    "$(awk '$1 > 0 { n++ } END { print n + 0 }' "$work/own_$1")"
}

stage_scenarios() {
  sample_period=0.1
  local base="$work/stages" i size install=(//tools/hooks:install)
  local bazel_flags=()
  if [[ "$isolation" == weakened ]]; then
    bazel_flags=(--config=weak-isolation)
    install+=(-- --weak-isolation)
  fi
  if [[ -n "$policy_base" ]]; then
    if [[ "$isolation" != weakened ]]; then install+=(--); fi
    install+=("--policy-base=$policy_base")
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
    observed_push "$base" edited
    expect_work edited
    echo "push edited $i: $(tail -1 "$work/push_edited")" >&2
  done
  expect_passed "$work/push_edited"

  : >"$work/push_exported"
  for i in $(seq 1 "$warm"); do
    printf '\n// BenchEdit%d is a measured edit.\nconst BenchEdit%d = %d\n' "$i" "$i" "$i" >>"$base/clone/$stage_file"
    in_stage "$base" git add "$stage_file"
    in_stage "$base" git commit -q --no-verify -m exported
    observed_push "$base" exported
    expect_work exported
    echo "push exported $i: $(tail -1 "$work/push_exported")" >&2
  done
  expect_passed "$work/push_exported"

  : >"$work/push_runtime"
  printf '\n// BenchEdits records the measured edits.\nvar BenchEdits []int\n' >>"$base/clone/$stage_file"
  for i in $(seq 1 "$warm"); do
    printf '\nfunc init() { BenchEdits = append(BenchEdits, %d) }\n' "$i" >>"$base/clone/$stage_file"
    in_stage "$base" git add "$stage_file"
    in_stage "$base" git commit -q --no-verify -m runtime
    observed_push "$base" runtime
    expect_work runtime
    if [[ "$(tail -1 "$work/own_runtime")" -eq 0 ]]; then
      echo "measure: an edit that changes what runs did not run the package's test" >&2
      exit 1
    fi
    echo "push runtime $i: $(tail -1 "$work/push_runtime"); tests $(tail -1 "$work/tests_runtime")" >&2
  done
  expect_passed "$work/push_runtime"

  : >"$work/push_rejected"
  for i in $(seq 1 "$warm"); do
    printf '\nfunc init() { panic("edit %d fails this package'"'"'s tests") }\n' "$i" >>"$base/clone/$stage_test"
    in_stage "$base" git add "$stage_test"
    in_stage "$base" git commit -q --no-verify -m rejected
    observed_push "$base" rejected
    echo "push rejected $i: $(tail -1 "$work/push_rejected")" >&2
    if ! grep -qF "//$stage_package" "$work/run.log"; then
      echo "measure: the push was rejected for another reason:" >&2
      tail -n 20 "$work/run.log" >&2
      exit 1
    fi
    if [[ "$(tail -1 "$work/own_rejected")" -eq 0 ]]; then
      echo "measure: a failing test left no new test.xml, so that probe cannot be trusted" >&2
      exit 1
    fi
    in_stage "$base" git reset -q --hard HEAD~1
  done
  expect_rejected "$work/push_rejected"

  : >"$work/push_docs"
  for i in $(seq 1 "$warm"); do
    echo "<!-- edit $i -->" >>"$base/clone/docs/quality-framework.md"
    in_stage "$base" git add docs/quality-framework.md
    in_stage "$base" git commit -q --no-verify -m docs
    observed_push "$base" docs
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
  "pre_push_edited_work": $(work_record edited),
  "pre_push_exported": $(stats "$work/push_exported"),
  "pre_push_exported_work": $(work_record exported),
  "pre_push_runtime": $(stats "$work/push_runtime"),
  "pre_push_runtime_work": $(work_record runtime),
  "pre_push_rejected": $(stats "$work/push_rejected"),
  "pre_push_rejected_work": $(work_record rejected),
  "pre_push_docs_only": $(stats "$work/push_docs"),
  "pre_push_docs_only_work": $(work_record docs),
  "limits": "Times are a whole git commit or git push through the real hooks, including prek. failed_runs equals n on the rejected scenarios by construction. pre_commit_N staged N new Go files, so it exercises the content checks that apply to Go and nothing for other languages. pre_push_docs_only edits no build input of a test and is the stage's floor, not feedback on edited code. Scenario names say what was edited. The *_work records say what was seen: spawned actions from each receipt, and tests_run, a count of test.xml files whose path or modification time the push changed, with caching on. That count is a proxy, not a cache-aware record of execution. A time with tests_run 0 is a compile or cache time and not feedback on behavior. A spawned action can be a compile alone. pre_push_runtime is the only passing scenario required to update a test result; each sample adds a new retained init function, and its result is scoped to that edit of one package and its dependents. Install and first runs have too few samples to set a cold contract. Install and first runs include downloads when the host's download cache is empty, so they depend on the link. Peak memory is sampled PSS of the command's and Bazel servers' process trees every 0.1 s and can miss short spikes."
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
if $edited_only; then cold=0; fi
for i in $(seq 1 "$cold"); do
  mkdir -p "$work/cold$i/home"
  new_clone "$work/cold$i/clone"
  timed "$work/cold$i/home" "$work/cold$i/clone" -- tools/verify.sh "${verify_args[@]}" --bootstrap-only >>"$work/bootstrap"
  timed "$work/cold$i/home" "$work/cold$i/clone" -- tools/verify.sh "${verify_args[@]}" >>"$work/first"
  stop_server "$work/cold$i/home" "$work/cold$i/clone"
  echo "cold $i: bootstrap $(tail -1 "$work/bootstrap"); first verify $(tail -1 "$work/first")" >&2
  expect_passed "$work/bootstrap"
  expect_passed "$work/first"
  chmod -R u+w "$work/cold$i"
  rm -rf "$work/cold$i"
done

mkdir -p "$work/warm/home"
new_clone "$work/warm/clone"
timed "$work/warm/home" "$work/warm/clone" -- tools/verify.sh "${verify_args[@]}" >"$work/warm_first"
echo "warm clone's first verify: $(cat "$work/warm_first")" >&2
expect_passed "$work/warm_first"
: >"$work/nochange"
: >"$work/edited"
if ! $edited_only; then
  for i in $(seq 1 "$warm"); do
    timed "$work/warm/home" "$work/warm/clone" -- tools/verify.sh "${verify_args[@]}" >>"$work/nochange"
    echo "warm unchanged $i: $(tail -1 "$work/nochange")" >&2
    expect_passed "$work/nochange"
  done
fi
for i in $(seq 1 "$warm"); do
  # The blank line keeps the file gofmt-clean: format-go is in the full stage, so this run must pass it.
  printf '\n// Edit %d forces a rebuild and a retest of this package.\n' "$i" >>"$work/warm/clone/quality/checks/suppression/scan.go"
  timed "$work/warm/home" "$work/warm/clone" -- tools/verify.sh "${verify_args[@]}" >>"$work/edited"
  echo "warm edited $i: $(tail -1 "$work/edited")" >&2
  expect_passed "$work/edited"
done
if $edited_only; then
  stop_server "$work/warm/home" "$work/warm/clone"
  mkdir -p "$(dirname "$out")"
  cat >"$out" <<JSON
{
  "schema_version": 3,
  "scope": "edited-only: not a whole record",
  "revision": "$revision",
  "tree": "$tree",
  "measured_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "isolation": "$isolation",
  "warm_first": $(stats "$work/warm_first"),
  "warm_edited": $(stats "$work/edited")
}
JSON
  echo "measure: wrote $out; evidence in $evidence" >&2
  cat "$out"
  exit 0
fi
git -C "$work/warm/clone" checkout --quiet -- .
timed "$work/warm/home" "$work/warm/clone" HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 \
  -- tools/verify.sh "${verify_args[@]}" --offline >"$work/nodownload"
echo "no download, incomplete by design: $(cat "$work/nodownload")" >&2
stop_server "$work/warm/home" "$work/warm/clone"

mkdir -p "$(dirname "$out")"
cat >"$out" <<JSON
{
  "schema_version": 3,
  "revision": "$revision",
  "tree": "$tree",
  "measured_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "isolation": "$isolation",
  "host_profile": {"os": "$os", "kernel": "$(uname -r)", "arch": "$(uname -m)", "cpus": $ncpu, "memory_gb": $mem_gb, "disk_free_gb": $disk_free_gb},
  "bootstrap": $(stats "$work/bootstrap"),
  "first_verify": $(stats "$work/first"),
  "warm_nochange": $(stats "$work/nochange"),
  "warm_edited": $(stats "$work/edited"),
  "no_download": {"complete": false, "exit": $(awk '{print $3}' "$work/nodownload"), "sample": $(stats "$work/nodownload")},
  "limits": "bootstrap includes downloads, so its time depends on the link. warm_nochange reruns every test but reuses build outputs; warm_edited rebuilds one Go package and its dependents. no_download is never a passing verifier and is not one of the samples: it skips networked provenance, refuses to build the base's evaluator, and proves only that Bazel fetched nothing and proxy-honoring tools reached no network; it is not a network-isolation claim. Peak memory is sampled PSS of the verifier and Bazel server process trees every 0.5 s and can miss short spikes."
}
JSON
echo "measure: wrote $out; evidence in $evidence" >&2
cat "$out"
