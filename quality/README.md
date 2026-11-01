# bff_template_quality

## Purpose

This module holds the shared, versioned quality checks for the BFF template. It is a separate
Bazel module, so consumers adopt and upgrade it through one version pin (specs/001 R-07). It never
depends on the application that uses it.

## What to read first

- [specs/001 contracts/check-registry.md](../specs/001-bff-template-foundation/contracts/check-registry.md)
  is the registry and runner contract.
- [docs/quality/classification.md](../docs/quality/classification.md) explains check classes.
- [specs/002 research.md](../specs/002-hermetic-skeleton/research.md) describes the M1 pins and
  checks.

## Contents (0.2.0)

| Path | What it is |
| --- | --- |
| `checks/suppression` | The `suppression-ban` Go check. Its stable codes S001–S012 are defined in `rules.go`, and each code has a defect and a corrected fixture. |
| `checks/coverage` | The coverage merger, and `floors`: `check` judges a run against the host's contract, and `compare`, built from a base, judges a tree's contract. |
| `checks/ciparity` | Validates a workflow against a closed allowlist policy. Unknown keys fail. |
| `proto` | The canonical `buf.yaml` and the compatibility baseline reader, which reads a declared commit from Git objects and fails closed. |
| `rules.textproto`, `proto` | The rule catalog, the one definition of each reusable rule, and its schema. |
| `defs.bzl` | `quality_binary`, which builds the runner for a host. |
| `hooks` | `install`, and `pin`, which keeps a consumer's two adapters on one commit. |
| `release` | Release notes from the catalog, and the tag record. See [RELEASING.md](RELEASING.md). |
| `runner` | The one runner: selection, snapshots, the bounded pool, the single Bazel invocation, receipts, deadlines and server custody. |
| `adapters/native.sh` | The one definition of how each native tool is invoked. |
| `analysis` | The nogo configuration: vet and staticcheck. |
| `tools` | Pinned native tools: `actionlint`, `buf`, `buildifier`, `gitleaks`, `prek`, `shellcheck`, `typos`, `uv`, `yq`, `zizmor`. They are exposed as `@bff_template_quality//tools:<name>`, and pinned by sha256 in `multitool.lock.json`. |

## How to use

In this repository, the root `MODULE.bazel` consumes the module with `local_path_override`. Here,
`registry.textproto` and `coverage_floors.json` are this repository's own host files. They sit under `quality/`
so that an accepted base can still read them, and a consumer reads neither.

Another repository pins one commit, in two adapters:

```starlark
# MODULE.bazel. This commit is the one pin.
bazel_dep(name = "bff_template_quality")
git_override(
    module_name = "bff_template_quality",
    commit = "<full SHA>",
    remote = "<remote>",
    strip_prefix = "quality",
)
```

```toml
# prek.toml. rev is written from MODULE.bazel, by the pin target's sync.
[[repos]]
repo = "<remote>"
rev = "<full SHA>"
hooks = [
  { id = "pre-commit", args = ["--bazel=<wrapper>"] },
  { id = "pre-push", args = ["--bazel=<wrapper>"] },
]
```

The hooks use prek's remote-hook interface, from `.pre-commit-hooks.yaml` at the commit's tree root. That is
not the shared stage contract of relentless-ci-python#25, which is unreleased.

The host provides:

| What | Notes |
| --- | --- |
| `//:quality` | `quality_binary` from `@bff_template_quality//:defs.bzl`, naming the paths below, and `GOFMT` from the Go SDK the host chose |
| A registry | Binds catalog rules with `rules { id: ... shard: ... }`, with its paths in `args`, `context` and `targets`. It has `escalate` and `shards`, and its own `checks` for its own scripts |
| A coverage contract, and a required tests list | Its own, with its own floors and legs |
| A Bazel wrapper | Takes startup options, and `--path` to print the verified binary |
| `.bazelrc` | The `weak-isolation` config if a host needs it, and the coverage generators, as in this repository's |

After changing the pin:

```bash
<wrapper> run --lockfile_mode=update @bff_template_quality//hooks:pin -- sync
<wrapper> mod deps --lockfile_mode=update
```

The `quality-pin` rule fails if the two adapters differ, or if `local_path_override` or a local hook stands in.
It reads only the form shown above, and refuses anything else by line, so `prek.toml` holds that one repository,
each hook sets only `id` and `args`, and `git_override` sets only the four keys shown. Blank lines and whole-line
comments are fine. A host's own checks go in its registry. A hook that overrides its entry never reaches the
rule at commit time, so `tools/verify.sh` is what catches it. The rule reads these two files, so a
`--override_module` flag or rc line is outside it.
A host that builds with `--config=weak-isolation` passes it to the `run` above as well.

The interface is not stable until it is declared so in [RELEASING.md](RELEASING.md).

## Owner

The template maintainers (see the root README).
