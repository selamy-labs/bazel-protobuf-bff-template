#!/usr/bin/env bash
# Feedback-cost measurement (specs/002-hermetic-skeleton FR-012, research R6).
#
#   tools/bench/measure.sh [--cold N] [--warm N] [--weak-isolation] [--out FILE]
#
# cold: a fresh clone with an empty HOME per run (no Bazel, Bazelisk or uv caches).
# warm: repeated runs in one clone with its caches kept.
# no-network: one warm run with all HTTP(S) egress sent to a dead proxy and --offline (SC-005).
# Peak memory sums RSS over the Bazel server's process tree, sampled every 0.5 s; short spikes can
# be missed and the record says so.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
cold=3
warm=5
weak=false
out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --cold) cold="$2"; shift 2 ;;
    --warm) warm="$2"; shift 2 ;;
    --weak-isolation) weak=true; shift ;;
    --out) out="$2"; shift 2 ;;
    *) echo "measure: unknown argument '$1'" >&2; exit 2 ;;
  esac
done
config=()
verify_args=()
isolation=full
if $weak; then
  config=(--config=weak-isolation)
  verify_args=(--weak-isolation)
  isolation=weakened
fi
revision="$(git -C "$root" rev-parse HEAD)"
ncpu="$(nproc)"
mem_gb="$(awk '/MemTotal/ {printf "%d", $2/1024/1024}' /proc/meminfo)"
os="$(awk -F= '/^ID=/{id=$2} /^VERSION_ID=/{v=$2} END{gsub(/"/,"",id); gsub(/"/,"",v); print id "-" v}' /etc/os-release)"
out="${out:-$root/docs/budgets/$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)-${ncpu}c.json}"
work="$(mktemp -d)"

tree_rss_kb() { # sum RSS of every process whose ancestry includes a process mentioning $1
  local marker="$1"
  ps -eo pid=,ppid=,rss=,args= | awk -v m="$marker" '
    { pid[NR]=$1; ppid[$1]=$2; rss[$1]=$3; if (index($0, m)) root[$1]=1 }
    END {
      total=0
      for (i in pid) { p=pid[i]; q=p; while (q > 1) { if (q in root) { total+=rss[p]; break } q=ppid[q] } }
      print total
    }'
}

run_measured() { # run_measured <home> <clone> <extra env...> -- <verify args...>; prints "seconds peak_kb rc"
  local home="$1" clone="$2"
  shift 2
  local envs=()
  while [[ "$1" != "--" ]]; do envs+=("$1"); shift; done
  shift
  local peak=0 sampler start end rc=0
  (while true; do
    tree_rss_kb "$home" >>"$work/samples"
    sleep 0.5
  done) &
  sampler=$!
  : >"$work/samples"
  start="$(date +%s.%N)"
  (cd "$clone" && env -i HOME="$home" PATH="$PATH" TERM=dumb USER="${USER:-}" GIT_CONFIG_NOSYSTEM=1 "${envs[@]}" \
    bazel run "${config[@]}" //:verify -- "$@" </dev/null >"$work/run.log" 2>&1) || rc=$?
  end="$(date +%s.%N)"
  kill "$sampler" 2>/dev/null || true
  wait "$sampler" 2>/dev/null || true
  peak="$(sort -n "$work/samples" | tail -1)"
  printf '%s %s %s\n' "$(awk -v a="$start" -v b="$end" 'BEGIN {printf "%.1f", b-a}')" "${peak:-0}" "$rc"
}

new_clone() { # new_clone <dir>
  git clone --quiet --no-local "file://$root" "$1"
  git -C "$1" checkout --quiet "$revision"
}

shutdown_in() { (cd "$2" && HOME="$1" bazel shutdown) >/dev/null 2>&1 || true; }

stats() { # stats <file of "seconds peak rc" lines> -> JSON fragment
  sort -n -k1,1 "$1" | awk '
    { s[NR]=$1; if ($2>p) p=$2; if ($3!=0) bad++ }
    END {
      n=NR; i50=int((n+1)/2); i95=int(n*0.95+0.999); if (i95<1) i95=1; if (i95>n) i95=n
      printf "{\"n\": %d, \"p50_s\": %s, \"p95_s\": %s, \"peak_rss_mb\": %d, \"failed_runs\": %d}", n, s[i50], s[i95], p/1024, bad+0
    }'
}

: >"$work/cold"
for i in $(seq 1 "$cold"); do
  mkdir -p "$work/cold$i/home"
  new_clone "$work/cold$i/clone"
  run_measured "$work/cold$i/home" "$work/cold$i/clone" -- "${verify_args[@]}" >>"$work/cold"
  shutdown_in "$work/cold$i/home" "$work/cold$i/clone"
  echo "cold $i: $(tail -1 "$work/cold")" >&2
  chmod -R u+w "$work/cold$i" && rm -rf "$work/cold$i"
done

mkdir -p "$work/warm/home"
new_clone "$work/warm/clone"
run_measured "$work/warm/home" "$work/warm/clone" -- "${verify_args[@]}" >/dev/null # prime caches
: >"$work/warm.txt"
for i in $(seq 1 "$warm"); do
  run_measured "$work/warm/home" "$work/warm/clone" -- "${verify_args[@]}" >>"$work/warm.txt"
  echo "warm $i: $(tail -1 "$work/warm.txt")" >&2
done
dead=(HTTP_PROXY=http://127.0.0.1:9 HTTPS_PROXY=http://127.0.0.1:9 http_proxy=http://127.0.0.1:9 https_proxy=http://127.0.0.1:9)
run_measured "$work/warm/home" "$work/warm/clone" "${dead[@]}" -- "${verify_args[@]}" --offline >"$work/nonet"
echo "no-network: $(cat "$work/nonet")" >&2
shutdown_in "$work/warm/home" "$work/warm/clone"
chmod -R u+w "$work" && rm -rf "$work/warm"

mkdir -p "$(dirname "$out")"
cat >"$out" <<EOF
{
  "schema_version": 1,
  "revision": "$revision",
  "measured_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "isolation": "$isolation",
  "host_profile": {"os": "$os", "kernel": "$(uname -r)", "arch": "$(uname -m)", "cpus": $ncpu, "memory_gb": $mem_gb},
  "scenario": "bazel run //:verify (full canonical verifier)",
  "cold": $(stats "$work/cold"),
  "warm": $(stats "$work/warm.txt"),
  "warm_no_network": $(stats "$work/nonet"),
  "limits": "peak RSS is sampled every 0.5 s over the Bazel server process tree and may miss short spikes; cold includes downloading Bazel, the Go SDK and all tools; network fetch time depends on the link"
}
EOF
rm -rf "$work"
echo "measure: wrote $out" >&2
cat "$out"
