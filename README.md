# Generic Bazel/Protobuf BFF Template

## Purpose

This is a neutral starter for contract-first web applications. A browser client and a
backend-for-frontend talk only through generated Protocol Buffer/Connect clients, and Bazel owns the
toolchains, code generation and tests. The aim is shared, versioned quality checks that run the same
way in Git hooks, in the local verifier and in CI. Hooks and shared-check distribution are not built
yet.

**Status: M1 to M4 exist locally.** The app works: `CreateNote` and `GetNote` run from a React page
through a Go server, tested in a pinned browser. M1 to M3 passed independent local Linux
verification. M4 awaits independent review, and [the evidence page](docs/evidence/m2-m4.md)
lists what has and has not been observed. macOS and hosted CI are unverified. The repository is
local-only and unpublished, and it ships no identity provider, so the production server refuses to
start until you add one ([bff/README.md](bff/README.md)).

## How to use and test

Prerequisites: Git, bash, curl and coreutils (GNU `timeout`, `sha256sum`, `tar`), on a Linux x86_64
host with glibc 2.36 or newer for the browser tests. Bazel itself, the Go SDK, Node, Python, a browser
and every check tool are fetched at pinned, checksummed versions; `tools/bootstrap/bazel.sh` verifies
the Bazel binary on every call. The verifier discards the caller's environment, so rc files, tool options and PATH helpers cannot
change a verdict. It does trust its own caches under `$HOME/.cache`: Bazel does not re-hash an
extracted install base.

```sh
tools/verify.sh
```

The command fails closed if the host cannot provide namespace sandboxing, for example on
Ubuntu 24.04 with the default AppArmor policy. The preflight prints the fixes. If you can't apply
them, opt in to reduced isolation explicitly; the result is labeled `isolation: weakened`:

```sh
tools/verify.sh --weak-isolation
```

To run the app, then open <http://127.0.0.1:5173>:

```sh
tools/bootstrap/bazel.sh run //tools/dev
```

On a host that needs reduced isolation, add `--config=weak-isolation` after `run`. This command uses
the development server, which accepts any test identity and listens only on loopback.

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

Spec Kit is pinned to `github/spec-kit` v1.2.3 (commit `5ea8b1cf`) and run through a pinned uv with hash-locked dependencies. To check
that the vendored scaffold matches that release:

```sh
tools/speckit/bootstrap.sh verify
```

## Owner

To be decided at M8 (proposal: Sélamy Labs). See research R-14.
