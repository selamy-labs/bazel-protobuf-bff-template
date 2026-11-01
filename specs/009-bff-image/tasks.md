# Tasks: BFF container image

**Input**: Design documents from `specs/009-bff-image/`
**Prerequisites**: plan.md, spec.md, research.md, contracts/image.md, quickstart.md
**Tests**: required by the spec. FR-007 makes a witness part of the feature.

## Phase 1: Setup

- [x] T001 Land spec, plan, research, contract, quickstart and matrix in `specs/009-bff-image/`
- [ ] T002 Add rules_oci and tar.bzl to `MODULE.bazel`, regenerate `MODULE.bazel.lock`, and confirm the existing verifier result is unchanged

## Phase 2: User Story 1 - One image from the build graph (Priority: P1)

**Goal**: one Bazel target gives an OCI layout holding only the production server.

**Independent Test**: `//bff/cmd/bff:image_test` reads the built image's content.

- [ ] T003 [US1] Write `bff/cmd/bff/image_test.go`: descriptor closure with digests and sizes checked against content, platform, user, entrypoint, exact file list, no `PT_INTERP`, no `DT_NEEDED`. Show it failing with no image
- [ ] T004 [US1] Add `layer`, `image`, `image_tarball` and `image_test` to `bff/cmd/bff/BUILD.bazel`, compatible with linux x86_64 only, with pure Go on the binary
- [ ] T005 [US1] Enroll the test in `tools/checks/required_tests.txt` with its leg
- [ ] T006 [US1] Named defects in `bff/cmd/bff/BUILD.bazel`, each with the restored control: uid 0, a second file, `bff-dev` packaged, a wrong entrypoint, a dynamic binary. For `bff/cmd/bff/image_test.go`, a blob that does not match its digest or size
- [ ] T007 [US1] Repeatability, run from a script outside the repository: two clones, two paths, two output bases, manifest digests compared from content. Record it in `docs/evidence/m8a.md`

**Checkpoint**: the image exists and its content is witnessed. This is the MVP.

## Phase 3: User Story 2 - The image behaves as the server does today (Priority: P1)

**Goal**: the image shows the current refusal, and no more.

**Independent Test**: the entrypoint from the image exits 1 with the exact text in `contracts/image.md`.

- [ ] T008 [US2] Extend `bff/cmd/bff/image_test.go`: run the entrypoint taken from the layer, assert exit 1, exact stderr, no listener
- [ ] T009 [US2] Record the image's content digest and `image_tarball` for an independent run in a real runtime on an authorized host, bound to that digest, in `docs/evidence/m8a.md`

## Phase 4: User Story 3 - One builder, stated everywhere (Priority: P2)

**Goal**: one image builder in every document.

**Independent Test**: read `design.md` D8 and section 9, R-13 and T083 in spec 001.

- [x] T010 [US3] Reword D8, section 9, R-13 and T083, and add FR-034, in `specs/001-bff-template-foundation/`
- [ ] T011 [P] [US3] State the healthy-deployment limit in `README.md` and `docs/quality/capability-states.md`

## Phase 5: Polish

- [ ] T012 [P] Add the image to the built-artifact scope in `docs/provenance.md`
- [ ] T013 Write `docs/evidence/m8a.md`, update `specs/README.md`, run the full verifier, freeze, hold for review

## Dependencies

- T002 comes before all code. T003 comes before T004. T004 comes before T005 to T009.
- User Story 2 needs User Story 1's image. User Story 3 needs no code.
- T013 comes last.

## Parallel Example

T011 and T012 touch different files, and either can run beside T003 to T009.

## Implementation Strategy

User Story 1 first, because everything else reads its image. Stop at its checkpoint if the lock change fails, and revise the research before going on.
