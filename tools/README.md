# tools

## Purpose

This directory holds repository orchestration scripts. The checks they run live in `quality/`.

## Contents

| Script | Role |
| --- | --- |
| `verify.sh` | The canonical verifier. CI runs exactly this. It scrubs the environment and runs `checks/verify_body.sh`. |
| `bootstrap/bazel.sh` | The pinned Bazel: sha256 checked on every call, host and user rc files disabled. |
| `bootstrap/preflight.sh` | Host capability preflight: prerequisites, disk, and namespace isolation. |
| `bootstrap/fresh_clone.sh` | Fresh-clone witness: an empty `HOME`, `PATH=/usr/bin:/bin`, and a path with spaces and regex characters. `--hostile` plants hostile settings and a real defect. |
| `bench/measure.sh` | Measures bootstrap, first verify, warm and edited-code cost and writes the result to `docs/budgets/`. |
| `policy/` | The WORKSPACE ban, the implicit-configuration ban, the exact `.bazelrc` allowlist, and the lockfile, checksum and ambient-rc witnesses. |
| `checks/` | `verify_body.sh`, `inventory.sh` (input discovery), `native.sh` (the one definition of each tool invocation), `test_evidence.sh` (judges the test log), `required_tests.txt`, and their witnesses. |
| `ci/` | `policy.json`, the closed workflow allowlist; `parity_test.sh` and `mutations.tsv`, its adversarial witnesses. |
| `speckit/` | The pinned Spec Kit bootstrap, its hash manifest and locked dependencies, and the offline manifest test. |
| `pins/` | Generator for `quality/multitool.lock.json`. |

## Owner

The template maintainers.
