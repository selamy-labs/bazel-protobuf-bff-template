# bff_template_quality

## Purpose

This module holds the shared, versioned quality checks for the BFF template. It is a separate
Bazel module, so consumers adopt and upgrade it through one version pin (specs/001 R-07). It never
depends on the application that uses it.

## What to read first

- [specs/001 contracts/check-registry.md](../specs/001-bff-template-foundation/contracts/check-registry.md)
  is the target registry design (M5).
- [specs/002 research.md](../specs/002-hermetic-skeleton/research.md) describes the M1 pins and
  checks.

## Contents (0.1.0)

| Path | What it is |
| --- | --- |
| `checks/suppression` | The `suppression-ban` Go check. Its stable codes S001–S012 are defined in `rules.go`, and each code has a defect and a corrected fixture. |
| `checks/ciparity` | Validates a workflow against a closed allowlist policy. Unknown keys fail. |
| `tools` | Pinned native tools: `actionlint`, `buildifier`, `gitleaks`, `shellcheck`, `typos`, `uv`, `yq`, `zizmor`. They are exposed as `@bff_template_quality//tools:<name>`, and pinned by sha256 in `multitool.lock.json`. |

## How to use

In this repository, the root `MODULE.bazel` consumes the module with `local_path_override`. Other
repositories pin an immutable commit with `git_override`.

The consumer chooses the Go SDK version, as this repository's root module does.

## Owner

The template maintainers (see the root README).
