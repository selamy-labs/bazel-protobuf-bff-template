# Generic Bazel/Protobuf BFF Template

## Purpose

This is a neutral starter for contract-first web applications. It contains a browser client and a
backend-for-frontend that talk only through generated Protocol Buffer/Connect clients. Bazel owns
the toolchains, code generation and tests. The quality checks are shared and versioned, and they
run identically in Git hooks, in the local verifier and in CI.

**Status: design review (M0).** This repository currently holds only a Spec Kit package; there is
no application code yet. It is local-only and unpublished.

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
