# Generic Bazel/Protobuf BFF Template

## Purpose

This is a neutral starter for contract-first web applications. It contains a browser client and a
backend-for-frontend that talk only through generated Protocol Buffer/Connect clients. Bazel owns
the toolchains, code generation and tests. The quality checks are shared and versioned, and they
run identically in Git hooks, in the local verifier and in CI.

**Status: M1 (hermetic skeleton).** The pinned toolchain, the shared quality module and the
canonical verifier exist. There is no application code yet; that starts in M2. The repository is
local-only and unpublished.

## How to use and test

Prerequisites: [Bazelisk](https://github.com/bazelbuild/bazelisk), Git, bash and coreutils. Bazel,
the Go SDK and every check tool are fetched at pinned, checksummed versions.

```sh
bazel run //:verify
```

The command fails closed if the host cannot provide namespace sandboxing, for example on
Ubuntu 24.04 with the default AppArmor policy. The preflight prints the fixes. If you can't apply
them, opt in to reduced isolation explicitly; the result is labeled `isolation: weakened`:

```sh
bazel run --config=weak-isolation //:verify -- --weak-isolation
```

Other entry points:

| Command | What it does |
| --- | --- |
| `tools/bootstrap/fresh_clone.sh` | Fresh-clone witness |
| `tools/bench/measure.sh` | Feedback-cost measurement; results go to `docs/budgets/` |

## What to read first

1. [Design for review](specs/001-bff-template-foundation/design.md)
2. [Constitution](.specify/memory/constitution.md)
3. [Spec of specs and milestone map](specs/001-bff-template-foundation/spec.md)
4. [Research, including the reference inspection](specs/001-bff-template-foundation/research.md)
5. [Tasks](specs/001-bff-template-foundation/tasks.md),
   [acceptance matrix](specs/001-bff-template-foundation/acceptance-matrix.md) and
   [issue map](specs/001-bff-template-foundation/issue-map.md)
6. [Provenance](docs/provenance.md) and [public-release review](docs/public-release-review.md)

## Spec Kit

Spec Kit is pinned to `github/spec-kit` v1.2.3 (commit `5ea8b1cf`) and run through `uvx`. To check
that the vendored scaffold matches that release:

```sh
tools/speckit/bootstrap.sh verify
```

## Owner

To be decided at M8 (proposal: Sélamy Labs). See research R-14.
