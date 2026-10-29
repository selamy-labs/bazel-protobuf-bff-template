# tools

## Purpose

This directory holds repository orchestration scripts. The checks they run live in `quality/`.

## Contents

| Script | Role |
| --- | --- |
| `verify.sh` | The canonical verifier, run as `bazel run //:verify`. CI runs exactly this. |
| `bootstrap/preflight.sh` | Host capability preflight: prerequisites, disk, and namespace isolation. |
| `bootstrap/fresh_clone.sh` | Fresh-clone witness: an empty `HOME` and a path containing a space. |
| `bench/measure.sh` | Measures cold, warm and no-network cost and writes the result to `docs/budgets/`. |
| `policy/` | The WORKSPACE ban, the Bzlmod policy test, and the lockfile witness. |
| `checks/` | Native-check fixtures and `run_on_files.sh`. |
| `ci/parity_test.sh` | Checks that CI runs only the verifier and that the gate aggregates every job. |
| `speckit/` | The pinned Spec Kit bootstrap, its hash manifest, and the offline manifest test. |

## Owner

The template maintainers.
