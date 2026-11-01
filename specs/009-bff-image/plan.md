# Implementation Plan: BFF container image

**Branch**: `009-bff-image` | **Date**: 2026-11-01 | **Spec**: [spec.md](spec.md)

## Summary

One `oci_image` target for the production binary, with no base, a declared mtree layer, and a standard-library Go witness. No registry, no runtime in the verifier, no deployment.

## Technical Context

**Language/Version**: Go 1.27.2 (pinned), Bazel 9.3.0
**Primary Dependencies**: rules_oci 2.3.0, tar.bzl 0.7.0, at the versions the lock resolves
**Testing**: `go_test` under Bazel, required by the verifier
**Target Platform**: linux/amd64
**Constraints**: no C++ toolchain, no container runtime in the verifier, no network beyond pinned fetches

## Constitution Check

| Principle | Result |
| --- | --- |
| I. Real slice, no vacuous surface | The image holds the real server, and its one claim is the real refusal. It succeeds at nothing it cannot do, so it is no stub. An adopter that assigns a verifier is its first consumer |
| III. Pinned | Rules are pinned in the module lock, and no floating base exists |
| IV. One policy | The witness joins the existing verifier as a required test |
| V. Policy cannot weaken itself | Unchanged |
| Governance: no live action | Nothing is pushed |

## Project Structure

```text
bff/cmd/bff/BUILD.bazel     image targets
bff/cmd/bff/image_test.go   witness
tools/checks/required_tests.txt
MODULE.bazel, MODULE.bazel.lock
docs/provenance.md
specs/001-bff-template-foundation/   D8, R-13, section 9, T083, FR-034
```

**Documentation**: `research.md`, `contracts/image.md`, `quickstart.md`, `acceptance-matrix.md`. There is no `data-model.md`, because the feature holds no data and its one entity is defined in the contract.

## Complexity Tracking

None.
