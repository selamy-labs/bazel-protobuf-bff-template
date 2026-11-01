"""What a repository uses to run this module's rules (spec 008 R8)."""

load("@rules_go//go:def.bzl", "go_binary")

# The tools the catalog's rules run, from this module. A host adds what only it can name: GOFMT, from
# the Go SDK it chose, and PRETTIER if it binds format-ts.
MODULE_TOOLS = {
    "ACTIONLINT": str(Label("//tools:actionlint")),
    "BUILDIFIER": str(Label("//tools:buildifier")),
    "FLOORS": str(Label("//checks/coverage:floors")),
    "FROM_BASE": str(Label("//adapters:from_base.sh")),
    "GITLEAKS": str(Label("//tools:gitleaks")),
    "PIN": str(Label("//hooks:pin.sh")),
    "SHELLCHECK": str(Label("//tools:shellcheck")),
    "SUPPRESSION": str(Label("//checks/suppression:suppression-ban")),
    "TEST_EVIDENCE": str(Label("//adapters:test_evidence.sh")),
    "TYPOS": str(Label("//tools:typos")),
    "ZIZMOR": str(Label("//tools:zizmor")),
}

def quality_binary(name, registry, contract, bazel, tools = {}, registry_path = None, **kwargs):
    """The one runner, bound to a host's registry, paths and tools.

    Args:
      name: the target's name. Hooks and the runner expect `//:quality`.
      registry: label of the host's registry.
      contract: the coverage contract's path in the repository.
      bazel: the host's Bazel wrapper, relative to the repository root. It must accept startup
        options, and `--path` to print the verified binary.
      tools: NAME to label, added to MODULE_TOOLS.
      registry_path: the registry's path in the repository, if it differs from the label.
      **kwargs: passed to go_binary.
    """
    every = dict(MODULE_TOOLS)
    every.update(tools)
    catalog = str(Label("//:rules.textproto"))
    adapter = str(Label("//adapters:native.sh"))
    go_binary(
        name = name,
        args = [
            "--catalog=$(rootpath %s)" % catalog,
            "--registry=$(rootpath %s)" % registry,
            "--registry-path=%s" % (registry_path or registry),
            "--contract=%s" % contract,
            "--bazel=%s" % bazel,
            "--adapter=$(rootpath %s)" % adapter,
        ] + ["--tool=%s=$(rootpath %s)" % (tool, label) for tool, label in sorted(every.items())],
        data = sorted(every.values()) + [adapter, catalog, registry],
        embed = [str(Label("//runner/cmd"))],
        **kwargs
    )
