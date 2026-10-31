#!/usr/bin/env bash
# Feedback-cost measurement (specs/002-hermetic-skeleton FR-012, T032, T033).
#
#   tools/bench/measure.sh [--cold N] [--warm N] [--weak-isolation] [--out FILE]
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
while [[ $# -gt 0 ]]; do
  case "$1" in
    --cold) cold="$2"; shift 2 ;;
    --warm) warm="$2"; shift 2 ;;
    --weak-isolation) verify_args=(--weak-isolation); isolation=weakened; shift ;;
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
out="${out:-$root/docs/budgets/$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)-${ncpu}c.json}"
disk_free_gb="$(df -Pk "$root" | awk 'NR==2 {printf "%d", $4/1024/1024}')"
work="$(mktemp -d)"
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT

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

timed() { # timed <home> <clone> <env assignments...> -- <verify args...>; prints "seconds peak_kb rc"
  local home="$1" clone="$2"
  shift 2
  local envs=()
  while [[ "$1" != "--" ]]; do envs+=("$1"); shift; done
  shift
  local start end rc=0 peak=0 now server roots
  start="$(date +%s.%N)"
  mkdir -p "$home/tmp"
  (cd "$clone" && exec env -i HOME="$home" TMPDIR="$home/tmp" PATH=/usr/bin:/bin USER="${USER:-$(id -un)}" TERM=dumb "${envs[@]}" \
    tools/verify.sh "$@" </dev/null >"$work/run.log" 2>&1) &
  local verifier=$!
  while kill -0 "$verifier" 2>/dev/null; do
    roots="$verifier"
    # The toolchain fixtures start their own server under TMPDIR, which is inside HOME here.
    server="$(find "$home" -maxdepth 7 -name server.pid.txt -exec cat {} + 2>/dev/null | tr '\n' ' ' || true)"
    roots+=" $server"
    now="$(tree_pss_kb "$roots")"
    if ((now > peak)); then peak=$now; fi
    sleep 0.5
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

: >"$work/bootstrap"
: >"$work/first"
for i in $(seq 1 "$cold"); do
  mkdir -p "$work/cold$i/home"
  new_clone "$work/cold$i/clone"
  timed "$work/cold$i/home" "$work/cold$i/clone" -- "${verify_args[@]}" --bootstrap-only >>"$work/bootstrap"
  timed "$work/cold$i/home" "$work/cold$i/clone" -- "${verify_args[@]}" >>"$work/first"
  stop_server "$work/cold$i/home" "$work/cold$i/clone"
  echo "cold $i: bootstrap $(tail -1 "$work/bootstrap"); first verify $(tail -1 "$work/first")" >&2
  chmod -R u+w "$work/cold$i"
  rm -rf "$work/cold$i"
done

mkdir -p "$work/warm/home"
new_clone "$work/warm/clone"
timed "$work/warm/home" "$work/warm/clone" -- "${verify_args[@]}" >/dev/null
: >"$work/nochange"
: >"$work/edited"
for i in $(seq 1 "$warm"); do
  timed "$work/warm/home" "$work/warm/clone" -- "${verify_args[@]}" >>"$work/nochange"
  echo "warm unchanged $i: $(tail -1 "$work/nochange")" >&2
done
for i in $(seq 1 "$warm"); do
  echo "// Edit $i forces a rebuild and a retest of this package." >>"$work/warm/clone/quality/checks/suppression/scan.go"
  timed "$work/warm/home" "$work/warm/clone" -- "${verify_args[@]}" >>"$work/edited"
  echo "warm edited $i: $(tail -1 "$work/edited")" >&2
done
git -C "$work/warm/clone" checkout --quiet -- .
timed "$work/warm/home" "$work/warm/clone" HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 \
  -- "${verify_args[@]}" --offline >"$work/nodownload"
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
