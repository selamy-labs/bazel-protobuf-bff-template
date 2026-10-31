# Generic Bazel/Protobuf BFF Template

## Purpose

This project is building a neutral starter for contract-first web applications. The goal is a
browser client and a backend-for-frontend that talk only through generated Protocol Buffer/Connect
clients, with Bazel owning the toolchains, code generation and tests, and with shared, versioned
quality checks that run identically in Git hooks, in the local verifier and in CI. Today only the
toolchain and verifier parts exist; the status below says exactly what does.

**Status: M1 (hermetic skeleton).** The pinned toolchain, the shared quality module and the
canonical verifier exist, and an independent review of them is being remediated (see
[review-remediation.md](specs/002-hermetic-skeleton/review-remediation.md)). There is no application
code yet; that starts in M2. The repository is
local-only and unpublished.

## How to use and test

Prerequisites: Git, bash, curl and coreutils (GNU `timeout`, `sha256sum`, `tar`). Bazel itself, the Go SDK, Python
and every check tool are fetched at pinned, checksummed versions; `tools/bootstrap/bazel.sh` verifies
the Bazel binary on every call. Nothing else on the host is used: the verifier discards the caller's
environment, so rc files, tool options and PATH helpers cannot change a verdict.

```sh
tools/verify.sh
```

The command fails closed if the host cannot provide namespace sandboxing, for example on
Ubuntu 24.04 with the default AppArmor policy. The preflight prints the fixes. If you can't apply
them, opt in to reduced isolation explicitly; the result is labeled `isolation: weakened`:

```sh
tools/verify.sh --weak-isolation
```

Other entry points:

| Command | What it does |
| --- | --- |
| `tools/bootstrap/fresh_clone.sh` | Fresh-clone witness; `--hostile` also plants rc files and a real defect |
| `tools/bench/measure.sh` | Feedback-cost measurement of bootstrap, first verify, warm and edited runs; results go to `docs/budgets/` |

`bazel` on your PATH is not used by the verifier. To run Bazel yourself with the same pin and rc
isolation, use `tools/bootstrap/bazel.sh`.

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
