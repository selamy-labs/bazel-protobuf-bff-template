#!/usr/bin/env bash
# The repository settings as a reviewed plan (spec 010 FR-009, FR-009a): format, validate, the pinned graph and
# the mock tests, with no reachable network and no credentials. The provider comes from a mirror assembled
# from the pinned zip and the lock's hash, so nothing is fetched. Nothing here applies anything.
#
#   governance.sh <tofu> <provider zip> <config dir>
#
# Exit 0 passed, 1 a finding, 11 nothing could be judged.
set -euo pipefail
tofu="$1"; zip="$2"; config="$3"
refuse() { echo "governance: $1" >&2; exit 11; }
[[ -x "$tofu" ]] || refuse "no tofu at $tofu"
[[ -f "$zip" ]] || refuse "no provider zip at $zip"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
hash="$(grep -oE 'h1:[A-Za-z0-9+/=]+' "$config/.terraform.lock.hcl" | head -1)"
[[ -n "$hash" ]] || refuse "the lock names no h1 hash"
mirror="$work/mirror/registry.opentofu.org/integrations/github"
mkdir -p "$mirror"
cp "$zip" "$mirror/terraform-provider-github_6.13.0_linux_amd64.zip"
printf '{"versions":{"6.13.0":{}}}\n' >"$mirror/index.json"
printf '{"archives":{"linux_amd64":{"url":"terraform-provider-github_6.13.0_linux_amd64.zip","hashes":["%s"]}}}\n' "$hash" >"$mirror/6.13.0.json"
cat >"$work/cli.tfrc" <<CLI
provider_installation {
  filesystem_mirror {
    path    = "$work/mirror"
    include = ["integrations/github"]
  }
}
CLI
cp -rL "$config" "$work/config"
export HOME="$work/home" TF_PLUGIN_CACHE_DIR="" TF_CLI_CONFIG_FILE="$work/cli.tfrc" HTTPS_PROXY=http://127.0.0.1:9 HTTP_PROXY=http://127.0.0.1:9 NO_PROXY="" GITHUB_TOKEN="" GH_TOKEN="" TF_INPUT=0
mkdir -p "$HOME"
rm -rf "$work/config/.terraform"
"$tofu" -chdir="$work/config" fmt -check -recursive >/dev/null || { echo "governance: the configuration is not formatted"; exit 1; }
# The declaration is read structurally, not counted by file name: no JSON configuration, no module, no import, no
# data source, exactly the two resources, and then the real graph that tofu builds from the loaded configuration.
if find "$work/config" -name '*.tf.json' -o -name '*.tofu.json' | grep -q .; then echo "governance: JSON configuration is not accepted here"; exit 1; fi
! grep -rqE '^\s*(import|data|module) ' "$work/config"/*.tf || { echo "governance: an import block, a data source or a module is present"; exit 1; }
[[ "$(cat "$work/config"/*.tf | grep -cE '^resource ')" == 2 ]] || { echo "governance: the resource graph is not the two declared resources"; exit 1; }
"$tofu" -chdir="$work/config" init -input=false -lockfile=readonly >/dev/null || refuse "init did not complete from the mirror"
"$tofu" -chdir="$work/config" validate >/dev/null || { echo "governance: the configuration is not valid"; exit 1; }
graph="$("$tofu" -chdir="$work/config" graph 2>/dev/null)" || refuse "the graph could not be built"
nodes="$(grep -oE '"[a-z_]+\.[A-Za-z0-9_-]+( \(expand\))?"' <<<"$graph" | grep -E '^"github_' | sed 's/ (expand)//' | LC_ALL=C sort -u | tr -d '"' | tr '\n' ' ')"
[[ "$nodes" == "github_repository.template github_repository_ruleset.main " ]] || { echo "governance: the loaded graph holds [$nodes], not the two declared resources"; exit 1; }
grep -qE '^\s*owner\s*=\s*var\.owner\s*$' "$work/config/main.tf" || { echo "governance: the provider is not bound to the pinned owner"; exit 1; }
"$tofu" -chdir="$work/config" test || { echo "governance: a mock test failed"; exit 1; }
echo "governance: one target, formatted, valid, two resources, mock tests passed, offline"
