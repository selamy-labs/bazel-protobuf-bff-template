#!/usr/bin/env bash
# Witness for image_release.sh: a wrapper that fails is a finding, a missing wrapper is a refusal, and a
# wrapper that passes yields one measurement naming a content-hashed digest.
set -euo pipefail
script="$PWD/$1"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0
expect() { local rc=0 out; out="$("$script" "$2" "$3" 2>&1)" || rc=$?; if [[ "$rc" == "$4" && "$out" == *"$5"* ]]; then echo "ok   $1"; else echo "FAIL $1: exit $rc: $out"; failures=$((failures + 1)); fi; }
expect "no wrapper" "$work/absent" weakened 11 "no Bazel wrapper"
printf '#!/usr/bin/env bash\nexit 3\n' >"$work/failing"; chmod +x "$work/failing"
expect "a failing witness" "$work/failing" weakened 1 ""
# a wrapper whose test passes and whose cquery names a layout with a real manifest
mkdir -p "$work/layout/blobs/sha256"; printf '{"x":1}' >"$work/m"; h="$(sha256sum "$work/m" | cut -d' ' -f1)"; mv "$work/m" "$work/layout/blobs/sha256/$h"
printf '{"manifests":[{"digest":"sha256:%s"}]}' "$h" >"$work/layout/index.json"
cat >"$work/passing" <<PASSING
#!/usr/bin/env bash
if [[ "\$1" == cquery ]]; then echo "$work/layout"; fi
exit 0
PASSING
chmod +x "$work/passing"
expect "a passing witness reports the digest" "$work/passing" full 0 "measurement: image sha256:$h"
printf 'tampered' >"$work/layout/blobs/sha256/$h"
expect "a manifest that does not hash to its digest" "$work/passing" full 1 "do not hash"
exit "$failures"
