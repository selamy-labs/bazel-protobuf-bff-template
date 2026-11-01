# Feature Specification: BFF container image

**Feature Branch**: `009-bff-image`
**Created**: 2026-11-01
**Status**: Draft
**Input**: Goal amendment 2026-11-01: Bazel and rules_oci own application images. Reconcile the Cloud Build image promise, and implement the smallest target with build, structure and startup evidence. Parent: spec 001 FR-034.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An adopter gets the production server as one image from the build graph (Priority: P1)

An adopter builds one Bazel target and receives an OCI image holding the production server and nothing else.

**Why this priority**: it is the whole feature, and it removes the only conflict with one image builder.

**Independent Test**: build the target from a clean clone and read the image's own config and layers.

**Acceptance Scenarios**:

1. **Given** a clean clone on the supported profile, **When** the target is built, **Then** an OCI layout results, with linux/amd64, a numeric non-root user, and the production binary as its entrypoint.
2. **Given** the image, **When** its layers are listed, **Then** they hold exactly that binary, and no development binary or test identity.
3. **Given** two clean builds in different paths, **When** their content digests are compared, **Then** they are equal.

### User Story 2 - The image behaves as the server does today (Priority: P1)

An adopter who starts the image with no verifier assigned sees the same refusal as the binary.

**Why this priority**: the image must not imply a supported deployment the server does not have.

**Independent Test**: run the entrypoint taken from the image, and in a real runtime on a host that has one.

**Acceptance Scenarios**:

1. **Given** the image as shipped, **When** it starts, **Then** it exits 1 with the exact refusal in [contracts/image.md](contracts/image.md), and serves nothing.
2. **Given** a change that packages a test verifier or a development binary, **When** the witnesses run, **Then** they fail.

### User Story 3 - One builder, stated everywhere (Priority: P2)

A reader of the design finds one image builder. A hosted pipeline, if one is ever added, invokes the Bazel target and publishes its digest.

**Why this priority**: the old text promises a second builder, and it would stay wrong without this.

**Independent Test**: read the design, research and task text that named Cloud Build as a builder.

**Acceptance Scenarios**:

1. **Given** the reconciled specs, **When** a reader looks for an image builder, **Then** Bazel with rules_oci is the only one, and Cloud Build may only invoke it.

### Edge Cases

- A host without a C or C++ compiler builds the image, because that host is the qualified profile.
- A linux/arm64 or macOS host does not produce a mislabeled image. The target is skipped as incompatible.
- `tools/verify.sh` needs no container runtime.
- The binary has no dynamic loader dependency, because the image has none to offer.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The image MUST be built only by the Bazel graph, with rules_oci at a pinned version in the module lock.
- **FR-002**: It MUST hold only the production server binary. No base image is used unless a requirement needs one.
- **FR-003**: Its config MUST state linux/amd64, a numeric non-root user and group, and an absolute entrypoint with no runfiles dependency.
- **FR-004**: The packaged binary MUST be statically linked, so that it starts with no loader in the image: its ELF has neither a program interpreter nor a dynamic dependency entry.
- **FR-005**: Its startup MUST show the server's current refusal exactly. No test identity or verifier is packaged to reach a healthy start.
- **FR-006**: The build MUST be repeatable: two clean plain builds in different paths give the same manifest digest, compared from actual content. That digest is the image's identity. A build in another configuration, such as the verifier's coverage run, witnesses content and behavior and not that digest.
- **FR-007**: A witness MUST read the built image's own descriptor closure, checking each digest and size against the content, then its config and layer. No filename or reported label is trusted. It MUST be required by the verifier. Neither it nor the verifier needs a container runtime or a registry.
- **FR-008**: What the image contains MUST join the built-artifact scope of the provenance inventory. An unknown license stays an open finding.
- **FR-009**: The design, research and tasks MUST state that a hosted pipeline only invokes this target and publishes its digest. It MUST NOT rebuild the image.
- **FR-010**: The documentation MUST state that a healthy production deployment is unsupported until a real verifier exists. No identity infrastructure is added. A built image or tarball MUST NOT be presented as startup or serving readiness: the acceptance result is the process's observed behavior.
- **FR-011**: Nothing is pushed, and no deployment platform or registry is added.

### Key Entities

- **Image**: the OCI layout that the target produces, identified by its manifest digest.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A clean clone on the qualified profile builds the image with no change to the existing verifier's other results.
- **SC-002**: The image holds one file, and each named defect fails the witness, with the restored control passing.
- **SC-003**: Two clean builds give one manifest digest.
- **SC-004**: The entrypoint from the image exits 1 with the exact refusal. A real runtime on an authorized host shows the same, labeled by that host, and bound to the same image content digest.

## Assumptions

- The production server needs no file from the image to reach its refusal. An adopter whose verifier needs CA roots or other files adds them with that verifier.
- Any later runtime asset, such as CA roots, is its own decision with its own pin and license.
- Only linux/amd64 is built, because that is the supported host. A second architecture waits for a consumer.
