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
tools/checks/image_release.sh      the plain-configuration gate, bound in quality/registry.textproto
MODULE.bazel, MODULE.bazel.lock
docs/provenance.md
specs/001-bff-template-foundation/   D8, R-13, section 9, T083, FR-034
```

**Documentation**: `research.md`, `contracts/image.md`, `quickstart.md`, `acceptance-matrix.md`. There is no `data-model.md`, because the feature holds no data and its one entity is defined in the contract.

## Complexity Tracking

| Exception | Why | Owner, condition |
| --- | --- | --- |
| A second Bazel invocation at `full`, by `image-release` | The stage's `coverage` invocation names transitioned output directories differently from a plain build, and the binary records them, so only a plain build produces the release artifact (research R6) | Spec 009. It stays while the Go toolchain records generated-source paths; it runs `after_bazel`, `exclusive`, uncached, and ordinary coverage and floors are untouched |

## Current amendment: rules_img and same-image serving

The historical plan above remains the provenance of the existing implementation. Implement the
single-image amendment in `spec.md` through T014–T021 below; it is not yet qualified.
The inspected current source pins Go 1.27.4, Bazel 9.3.0 and tar.bzl 0.10.4; the earlier
Technical Context records the original implementation rather than the present toolchain.
Keep the runtime-free source/full witnesses separate from real container acceptance. The same
canonical image acceptance command must run on an authorized runtime host and be required in
GitHub CI; source-only full verification cannot stand in for that container result.

1. Verify a compatible exact `rules_img` pin with this repository's Bazel 9.3.0, MODULE graph and
   locked toolchains in an owned output base. The upstream v0.3.26 release is a candidate, not a
   compatibility result. Use Bzlmod only; preserve nested quality-root locks and third-party history.
2. Keep the existing generated Connect services, production OIDC verifier and owned persistence.
   Add the smallest static-asset serving composition needed for a single origin; reuse the existing
   `PORT` precedence and local signed issuer fixture. Keep test identity outside the production image.
3. Migrate the existing image/load/push interfaces together where consumed. Preserve content/digest,
   non-root/static-runtime and no-verifier controls; update the old one-file expectation only for
   the required built UI/runtime content. No parallel image builder remains after adoption.
4. Enroll real image runtime and protocol proof in required GitHub CI, with no cloud credentials.
   Bind structure, startup, UI and Connect observations to one actual built image digest. Keep the
   canonical policy evaluator, floors and source-only tests unchanged.
5. Update current builder/provenance and affected publishing adapters, then qualify exact source,
   hosted/main artifact and distinct FUTURE through the existing owners and protections.

Sources: [rules_img v0.3.26](https://github.com/bazel-contrib/rules_img/releases/tag/v0.3.26),
[Cloud Run container contract](https://docs.cloud.google.com/run/docs/container-contract).
A release declaration does not prove compatibility. Provider configuration, issuer registration,
publication and production approval remain separate from this image source change.
