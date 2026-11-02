#!/usr/bin/env bash
# vulnerabilities (spec 011 T023): govulncheck in binary mode against the production server binary, the hosted
# boundary itself, with a vulnerability database the registry names. A reachable vulnerability is a finding; the
# database is fetched when the check runs, so the result holds for the database as of that run, which the
# measurement states, and the check is declared as needing the network. Source mode is not used: it needs `go
# list` over the module, which this Bazel-only tree cannot give for its generated packages.
#
#   vulnerabilities.sh <Bazel wrapper> <full|weakened> <govulncheck> <database url> <binary label>
#
# The binary is read at the output Bazel names for the label, never at a convenience symlink: the target
# transitions configuration, and the symlink can hold an older build.
#
# Exit 0 passed, 1 a finding, 11 nothing could be judged.
set -euo pipefail
wrapper="$1"
isolation="$2"
govulncheck="$3"
database="$4"
label="$5"
refuse() { echo "vulnerabilities: $1" >&2; exit 11; }
[[ -x "$wrapper" ]] || refuse "no Bazel wrapper at $wrapper"
[[ -x "$govulncheck" ]] || refuse "no govulncheck at $govulncheck"
[[ "$database" == https://* || "$database" == file://* ]] || refuse "the database must be an https or file URL, not '$database'"
if [[ "$database" == file://* && ! -f "${database#file://}/index/db.json" ]]; then refuse "no database index at $database"; fi
flags=(--noshow_progress)
if [[ "$isolation" == weakened ]]; then flags+=(--config=weak-isolation); fi
"$wrapper" build "${flags[@]}" "$label" </dev/null >/dev/null 2>&1 || refuse "the binary $label did not build"
binary="$("$wrapper" cquery "${flags[@]}" --output=files "$label" </dev/null 2>/dev/null | tail -n 1)"
[[ -n "$binary" && -f "$binary" ]] || refuse "the output of $label was not found"
work="$(mktemp -d)"
# Go leaves a module cache read-only, so the tree is made writable before it is removed.
trap 'chmod -R u+w "$work" 2>/dev/null || true; rm -rf "$work"' EXIT
# `govulncheck -version` runs `go env GOVERSION` when a go command is on PATH, and that go would honor go.mod by
# downloading a toolchain into HOME. The scan reads the binary's own version, so nothing here needs one.
export GOTOOLCHAIN=local
# govulncheck links the Go telemetry library, which reads no environment variable for this: only its mode file
# turns it off. Left on, it starts a child that outlives the scan and writes under the configuration directory
# while the trap removes it, and that directory is the host's whenever XDG_CONFIG_HOME is set.
export XDG_CONFIG_HOME="$work/.config"
mkdir -p "$XDG_CONFIG_HOME/go/telemetry"
echo off >"$XDG_CONFIG_HOME/go/telemetry/mode"
updated="$(HOME="$work" "$govulncheck" -version -db "$database" 2>&1 | sed -nE 's/^DB updated: (.*)$/\1/p' | head -n 1)"
[[ -n "$updated" ]] || refuse "the database at $database could not be read"
rc=0
HOME="$work" "$govulncheck" -mode=binary -db "$database" "$binary" >"$work/out" 2>"$work/err" || rc=$?
case "$rc" in
  0 | 3) ;;
  *) cat "$work/err" >&2; refuse "govulncheck exited $rc" ;;
esac
findings=0
id=""
while IFS= read -r line; do
  case "$line" in
    "Vulnerability #"*) id="${line#*: }"; id="${id%% *}"; findings=$((findings + 1)) ;;
    *"Found in: "* | *"Fixed in: "* | *"#"[0-9]*": "*)
      [[ -n "$id" ]] && echo "vulnerabilities $label: $id: $(sed -E 's/^ +//' <<<"$line")" ;;
    "    "[A-Z]*) [[ -n "$id" ]] && echo "vulnerabilities $label: $id: $(sed -E 's/^ +//' <<<"$line")" ;;
  esac
done <"$work/out"
echo "measurement: vulnerabilities $label: $findings reachable against $database updated $updated"
[[ "$findings" -eq 0 && "$rc" -eq 0 ]]
