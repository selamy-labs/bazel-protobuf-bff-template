# tools

## Purpose

This directory holds repository orchestration scripts. The checks they run live in `quality/`.

## Contents

| Script | Role |
| --- | --- |
| `verify.sh` | The canonical verifier. CI runs exactly this. It scrubs the environment and runs `checks/verify_body.sh`, which proves the host, builds `//:quality` and hands over to the registry. |
| `bootstrap/bazel.sh` | The pinned Bazel: sha256 checked on every call, host and user rc files disabled. |
| `bootstrap/preflight.sh` | Host capability preflight: prerequisites, disk, the Linux glibc floor, and namespace isolation. |
| `bootstrap/fresh_clone.sh` | Fresh-clone witness: an empty `HOME`, `PATH=/usr/bin:/bin`, and a path with spaces and regex characters. `--hostile` plants hostile settings and a real defect. |
| `bench/measure.sh` | Measures bootstrap, first verify, warm and edited-code cost, or with `--stages-only` the lifecycle stages, and writes the result to `docs/budgets/`. |
| `policy/` | The WORKSPACE ban, the implicit-configuration ban, the exact `.bazelrc` allowlist, and the lockfile, checksum, ambient-rc and no-compiler witnesses. |
| `checks/` | `verify_body.sh`, `test_evidence.sh` (judges the test log), `required_tests.txt`, and their witnesses. Tool invocations are in `quality/adapters/native.sh`, and input discovery is in the runner. |
| `hooks/` | The lifecycle hooks: `stage.sh`, `install.sh`, the tool list `tools.bzl`, `fixtures_test.sh` and its Bazel stand-in, and `e2e.sh`. See [docs/quality/hooks.md](../docs/quality/hooks.md). |
| `ci/` | `policy.json`, the closed workflow allowlist; `parity_test.sh` and `mutations.tsv`, its adversarial witnesses. |
| `speckit/` | The pinned Spec Kit bootstrap, its hash manifest and locked dependencies, and the offline manifest test. |
| `proto/` | `buf_generate`, the build-time code generation rule, and the schema probes. |
| `browser/` | The pinned browser, its shared libraries, fonts and font configuration. |
| `dev/` | `//tools/dev`, which runs the app locally. |
| `pins/` | Generators for `quality/multitool.lock.json` and `browser/debs.bzl`. |

## Owner

The template maintainers.
