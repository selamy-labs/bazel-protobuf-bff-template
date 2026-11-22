# Tasks: BFF container image

**Input**: Design documents from `specs/009-bff-image/`
**Prerequisites**: plan.md, spec.md, research.md, contracts/image.md, quickstart.md
**Tests**: required by the spec. FR-007 makes a witness part of the feature.

## Phase 1: Setup

- [x] T001 Land spec, plan, research, contract, quickstart and matrix in `specs/009-bff-image/`
- [x] T002 Add rules_oci and tar.bzl to `MODULE.bazel`, regenerate `MODULE.bazel.lock`, and confirm the existing verifier result is unchanged

## Phase 2: User Story 1 - One image from the build graph (Priority: P1)

**Goal**: one Bazel target gives an OCI layout holding only the production server.

**Independent Test**: `//bff/cmd/bff:image_test` reads the built image's content.

- [x] T003 [US1] Write `bff/cmd/bff/image_test.go`: descriptor closure with digests and sizes checked against content, platform, user, entrypoint, exact file list, no `PT_INTERP`, no `DT_NEEDED`. Show it failing with no image
- [x] T004 [US1] Add `layer`, `image`, `image_tarball` and `image_test` to `bff/cmd/bff/BUILD.bazel`, compatible with linux x86_64 only, with pure Go on the binary
- [x] T005 [US1] Enroll the test in `tools/checks/required_tests.txt` with its leg
- [x] T006 [US1] Named defects in `bff/cmd/bff/BUILD.bazel`, each with the restored control: uid 0, a second file, `bff-dev` packaged, a wrong entrypoint, a dynamic binary. For `bff/cmd/bff/image_test.go`, a blob that does not match its digest or size
- [x] T007 [US1] Repeatability, run from a script outside the repository: two clones, two paths, two output bases, manifest digests compared from content. Record it in `docs/evidence/m8a.md`

**Checkpoint**: the image exists and its content is witnessed. This is the MVP.

## Phase 3: User Story 2 - The image behaves as the server does today (Priority: P1)

**Goal**: the image shows the current refusal, and no more.

**Independent Test**: the entrypoint from the image exits 1 with the exact text in `contracts/image.md`.

- [x] T008 [US2] Extend `bff/cmd/bff/image_test.go`: run the entrypoint taken from the layer, assert exit 1, exact stderr, no listener
- [x] T009 [US2] Record the image's content digest and `image_tarball` for an independent run in a real runtime on an authorized host, bound to that digest, in `docs/evidence/m8a.md`

## Phase 4: User Story 3 - One builder, stated everywhere (Priority: P2)

**Goal**: one image builder in every document.

**Independent Test**: read `design.md` D8 and section 9, R-13 and T083 in spec 001.

- [x] T010 [US3] Reword D8, section 9, R-13 and T083, and add FR-034, in `specs/001-bff-template-foundation/`
- [x] T011 [P] [US3] State the healthy-deployment limit in `README.md` and `docs/quality/capability-states.md`

## Phase 5: Polish

- [x] T012 [P] Add the image to the built-artifact scope in `docs/provenance.md`
- [x] T013 Write `docs/evidence/m8a.md`, update `specs/README.md`, run the full verifier, freeze, hold for review

- T013: frozen at `05d2545`; the author's full verifier passed there, and an independent focused qualification of that revision is pending.

## Dependencies

- T002 comes before all code. T003 comes before T004. T004 comes before T005 to T009.
- User Story 2 needs User Story 1's image. User Story 3 needs no code.
- T013 comes last.

## Parallel Example

T011 and T012 touch different files, and either can run beside T003 to T009.

## Implementation Strategy

User Story 1 first, because everything else reads its image. Stop at its checkpoint if the lock change fails, and revise the research before going on.

## Phase 6: One rules_img image with a real serving journey

Historical T001–T013 and their evidence stay intact. These tasks implement the superseding
single-image amendment, not a retroactive qualification of that older image.

- [x] T014 Reconcile spec009, its contract/matrix and parent spec001 FR-034 for rules_img and same-image UI/Connect acceptance before implementation. No scaffold change.
- [x] T015 Probe compatible exact rules_img/Bazel9.3.0 resolution, migrate MODULE/lock and image/load/push consumers together, preserve linux/amd64/non-root/minimal runtime, and update provenance. FR-012; IMG-12.
- [x] T016 Add causal RED/GREEN public HTTP controls for static UI assets and existing generated Connect success/typed failure on the production composition's same origin and `PORT`; retain real OIDC refusal/auth-negative controls. FR-013/FR-015; IMG-13/IMG-15.
- [x] T017 Implement only the owned static-serving composition and package its built assets with the production server. No development runtime, test keys or shared BFF runtime. FR-013; IMG-13.
- [ ] T018 Build the actual OCI image and run that same artifact in a real runtime; verify structure, startup, UI and generated-client Connect success/typed failure and bad-bearer rejection. Record actual digest and planted missing-asset/wrong-PORT/broken-RPC failures with restored controls. FR-014/FR-015; IMG-14/IMG-15.
- [ ] T019 Enroll required credentialless GitHub CI image acceptance and existing publisher digest reuse; reconcile canonical CI parity/inventory and migration docs, including spec012's historical separate Pages frontend without weakening gates or adding live authority. FR-014/FR-016; IMG-14/IMG-16.
- [x] T019a Pin the production binary’s public fallback trust roots and provenance; prove default trust without system CA files and actual untrusted HTTPS refusal, retaining the same minimal image inventory. FR-013/FR-015; IMG-13/IMG-15.
- [x] T019b Reconcile spec012 typed frontend inputs for the selected same-origin image while preserving historical Pages fixtures; bind the existing browser client ID in service source and verify both profiles with unchanged offline guards. FR-013/FR-016; IMG-13/IMG-16.
- [x] T019c Reconcile the existing closed Bazel flag allowlist, real strict-lock negative and required-test leg inventory with the rules_img graph; retain all checks and prove the focused consumer controls. FR-012/FR-014; IMG-12/IMG-14.
- [ ] T020 Qualify the exact source through unchanged canonical full verification, three independent exact-head reviews, native required CI, normal protected merge and actual-main image/check evidence. No prior image approval transfer.
- [ ] T021 Complete distinct focused FUTURE inspection; merge only justified cleanup or retain bounded no-change acceptance before the next feature.

Dependencies: T014 → T015 → T016 → T017 → T018 → T019 → T020 → T021.
The optional cloud publishing/deployment profile retains its separate operator and approval gates.

T016/T017 local source increment: causal public HTTP 404 → built assets/config success; exact
image inventory missing UI → server plus production index/main bundle; signed local issuer,
real production browser/generated client success, typed owner-isolation failure, absent bearer
and wrong audience rejection all pass. The browser process uses `PORT` without an address flag,
and asserts the actual all-interface production/verified ready event. These are explicit
weak-isolation source/browser and OCI-content witnesses. T018's actual-container same-artifact
journey, its negative controls and T019–T021 remain pending; no hosted readiness is claimed.

T018/T019 source increment: the actual-archive/immutable-ID container harness, shared generated
browser journey, runtime readback controls and mandatory existing-e2e CI invocation are implemented.
Local contract/source-browser/lint/parity/inventory and runner/archive builds pass. Actual Docker
execution is pending on the authorized disposable CI host; no T018/T019 checkbox or acceptance
row is closed by the source witnesses. HTTPS trust roots and final migration docs remain open.

T019a local RED/GREEN: the public trust anchor failed without the production fallback import;
with the pinned binary-only import it verifies through default roots despite absent system CA
locations. A real untrusted HTTPS fixture is refused with UnknownAuthorityError. The static
image inventory/refusal, production config, source browser and inventory targets pass. Actual
container and final source/CI/review/delivery qualification remain separate.

T019 publisher increment: the real script now uses image-release's wrapper/isolation/parser
interface, requires actual-image runtime success before registry login, and compares the accepted
runtime manifest to the plain witnessed digest. Offline transport-only tests exercise its positive
path and reject runtime failure, changed identity and missing identity before any registry write.
The release retains runtime proof and the embedded dependency notice. No real publication or
actual container acceptance is inferred; T019b and T018/T020/T021 remain open.

T019b local source proof: the new public same-origin profile initially failed the Pages-only
funnel. The selected profile now requires root UI and RPC equal to runtime.allowed_origin, with
negative cross-origin/mount controls. Historical Pages fixtures still pass. Offline mock plans
and the full existing adversarial suite pass, including a newly planted omitted browser-client
binding. The source production-start witness initially reported the new declared environment
key missing from its own table; adding the exact existing audience binding restores the whole
inputs test. Only that changed Go target was rerun; the passing offline suite was retained.
Actual image CI and full exact-source FEATURE/main/FUTURE gates remain pending.

T019c local consumer qualification: the first normal full source push refused the closed flag
allowlist, two test-leg entries and a strict-lock negative whose selected version was already
transitively recorded. Only the two disabled image guards are admitted; four causal policy
mutations still reject their removal or enablement. Both helper tests remain required in the
existing e2e package leg. The real strict-lock fixture now first verifies its chosen registry
file is absent, observes lock refusal without lock rewriting, then restores and succeeds.
The whole existing checksum, ambient environment, compiler, hidden-worktree and analyzer
controls pass. Focused config-policy and verifier-body tests pass; final full/push/CI and
actual-container acceptance remain pending. No checker or threshold is relaxed.


- [x] T019d Bound the publication credential to the write-scoped `publish` job: move the plain
  build, `image_release.sh`, and actual-container browser witness (`image_runtime.sh`) into a
  credentialless `prepare.sh` step inside the read-only `verify` job; hand off the verified OCI
  layout archive and digest via `HANDOFF.sha256`; and publish by verified digest through the
  maintained pinned `oras` CLI (`tools/release/oras.sh` 1.3.0) with registry digest readback,
  failing any mismatch before registry login or release creation. FR-016 / IMG-16.

T019d local source witness: `publish_runtime_test.sh` exercises `prepare.sh` refusals (`GITHUB_TOKEN`,
`GH_TOKEN`, missing receipt, runtime failure/mismatch/absent), `publish.sh` pre-login tamper
refusals (`HANDOFF.sha256`, digest, revision, stray file, re-hashed archive substitution, unset
token, resolve mismatch), zero build/Docker/runtime invocation in publish, and offline `oras.sh`
checksum verification. These are offline/local transport controls, not hosted CI or live GHCR
publication acceptance. Current full verification, exact-head reviews, hosted CI, protected
integration and main/FUTURE remain required.
