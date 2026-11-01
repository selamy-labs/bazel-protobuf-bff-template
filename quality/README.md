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

## Contents (0.1.0)

| Path | What it is |
| --- | --- |
| `checks/suppression` | The `suppression-ban` Go check. Its stable codes S001–S012 are defined in `rules.go`, and each code has a defect and a corrected fixture. |
| `checks/coverage` | The coverage merger, and `floors`: `check` judges a run against `quality/coverage_floors.json`, and `compare`, built from a base, judges a tree's contract. |
| `checks/ciparity` | Validates a workflow against a closed allowlist policy. Unknown keys fail. |
| `proto` | The canonical `buf.yaml` and the compatibility baseline reader, which reads a declared commit from Git objects and fails closed. |
| `registry.textproto`, `proto` | The one list of checks and its schema. |
| `runner` | The one runner: selection, snapshots, the bounded pool, the single Bazel invocation, receipts, deadlines and server custody. |
| `adapters/native.sh` | The one definition of how each native tool is invoked. |
| `analysis` | The nogo configuration: vet and staticcheck. |
| `tools` | Pinned native tools: `actionlint`, `buf`, `buildifier`, `gitleaks`, `prek`, `shellcheck`, `typos`, `uv`, `yq`, `zizmor`. They are exposed as `@bff_template_quality//tools:<name>`, and pinned by sha256 in `multitool.lock.json`. |

## How to use

In this repository, the root `MODULE.bazel` consumes the module with `local_path_override`. Pinning it from
another repository with `git_override` is planned for M7, and it does not work yet: the module still assumes
host paths such as `quality/registry.textproto`. The interface is not stable.

The consumer chooses the Go SDK version, as this repository's root module does.

## Owner

The template maintainers (see the root README).
