# Acceptance matrix: BFF image

Historical layers: off, loc, hum, run (a real runtime on an authorized host). The original rows
below do not claim hosted or deployed acceptance. The current amendment adds required actual-image
GitHub CI evidence; those new rows are pending. Each negative needs its positive first.

| Req            | Witness                                                          | Valid                                                                                                                       | Defect → expected                                                                                                                                                       | Class     | Layer    |
| -------------- | ---------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- | -------- |
| FR-001         | Lock, review                                                     | one builder, pinned                                                                                                         | a Dockerfile or a second builder → review rejects                                                                                                                       | S         | hum      |
| FR-002         | `image_test`; Bazel analysis                                     | exactly `/bff`                                                                                                              | a second file → `image_test` fails; `bff-dev` → refused at analysis, because it is `testonly`                                                                           | G         | off      |
| FR-003         | `image_test`                                                     | linux/amd64, numeric non-root, absolute entrypoint                                                                          | uid 0, a user name, a wrong entrypoint → fails                                                                                                                          | G         | off      |
| FR-004         | `image_test`; a private-clone control                            | packaged ELF has no `PT_INTERP` and no `DT_NEEDED`                                                                          | the host's shell packaged as `/bff` → fails, naming `PT_INTERP`, `PT_DYNAMIC` and `libc.so.6`; a host without a dynamic `/bin/sh` → the witness fails rather than skips | G         | off, loc |
| FR-005         | `image_test`; real runtime run, bound to the same content digest | exit 1, exact refusal, nothing served                                                                                       | a healthy start, or a changed message → fails                                                                                                                           | G         | off, run |
| FR-006         | Repeatability script                                             | one digest from two paths                                                                                                   | a timestamp or path in a layer → digests differ                                                                                                                         | G         | loc      |
| FR-007         | Verifier, `image_test`, `image-release`                          | required, with no runtime; descriptor closure checked from content; the plain build witnessed and its digest in the receipt | the test unlisted → `test-inventory` fails; a blob that does not match its digest or size → fails; a failing plain witness → `image-release` fails                      | G         | off      |
| FR-008         | Review here; the inventory check at spec 010 T004                | image contents listed                                                                                                       | an unlisted runtime dependency → review rejects here, and the check fails at 010                                                                                        | S, then G | hum, off |
| FR-009, FR-010 | Review                                                           | one builder, limit stated                                                                                                   | a second builder, or a health claim → review rejects                                                                                                                    | S         | hum      |
| FR-011         | Review                                                           | nothing pushed                                                                                                              | a push step or registry → review rejects                                                                                                                                | S         | hum      |

## Single-image amendment witnesses (local source progress; runtime qualification pending)

These rows supersede refusal-only and binary-only completion claims for the new candidate.
The historical rows remain evidence for the prior image. No row is currently marked accepted.

| ID     | Requirement | Positive witness                                                                                                  | Negative witness                                                                                       | Layer                                               |
| ------ | ----------- | ----------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ | --------------------------------------------------- |
| IMG-12 | FR-012      | Exact compatible rules_img Bzlmod pin/lock; existing public image consumers resolve; linux/amd64/non-root         | Unpinned or wrong platform, stale rules_oci production consumer, unexpected runtime file rejected      | Local/full/CI/review                                |
| IMG-13 | FR-013      | Same image serves built UI and Connect from one origin using `PORT`, without host assets or runfiles              | Missing asset, host-only file dependency, localhost-only/wrong-port listener fails the runtime journey | Actual container/CI                                 |
| IMG-14 | FR-014      | Built OCI descriptor closure plus real same-digest runtime startup, UI and generated-client success/typed failure | Corrupt blob, wrong image, broken RPC or false digest binding fails                                    | Actual container/required CI                        |
| IMG-15 | FR-015      | Existing external signed local issuer enables authenticated journey; missing verifier refuses startup             | No bearer, invalid signature/issuer rejected; packaged test key or bypass rejected                     | Local/protocol/container/CI/review                  |
| IMG-16 | FR-016      | Configured publisher uses the tested digest; app owns selectively adopted source                                  | Rebuild/substituted digest, shared runtime dependency or missing live approval refused                 | Offline adapter/review; live operator gate separate |

Evidence must name exact source/policy, terminal command, runtime, digest and complete test inventory.
Passing source tests, extracting a binary, or having an image tarball alone cannot satisfy IMG-14.

T015 local builder/consumer proof: exact rules_img 0.3.26 resolves in the current graph;
production image/load/push build, uncached content/refusal controls and offline publisher
sink pass with one matching manifest. Exported layout contains no external blob links.
This is explicitly weak-isolation local evidence, with no real container or hosted execution.
IMG-12 remains pending final full/CI qualification; IMG-13–IMG-16 remain pending the serving journey.

T016/T017 local progress: causal HTTP and OCI-content RED/GREEN, complete config/server/
transport tests, production bundle inspection, and three real source-browser/signed-issuer
journeys pass. The browser verifies actual non-default `PORT` binding and same-origin generated
RPC success/typed failure/auth rejection. This is not the same-image container proof; IMG-13–
IMG-15 still need T018 and required CI. No complete amendment row is accepted yet.

T018/T019 source implementation now names `//web/e2e:image_runtime` through
`tools/checks/image_runtime.sh` in the required e2e workflow body, sharing the production browser
journey with the source witness. Runtime readback unit tests and the planted deadline source probe
are preparation evidence only. IMG-13–IMG-15 require actual same-artifact container execution and
all causal controls; IMG-16 and final FEATURE/main/FUTURE qualification remain pending.

T019a adds a bounded trust-content witness to IMG-13/IMG-15: with both system CA locations
unavailable before initialization, a public embedded trust anchor verifies through default
roots, while an actual untrusted loopback HTTPS endpoint is refused. The image file inventory
must remain unchanged. This does not certify any configured provider or the actual container.

T019c local IMG-12/IMG-14 consumer controls: the closed policy admits exactly the two disabled
rules_img settings and rejects each removed or enabled setting. The two runtime helper unit
tests remain required in the existing e2e leg, matching the unchanged package planner. The
actual strict-lock fixture rejects an unrecorded registry file without rewriting the lock,
then passes after restoration; all existing toolchain negative/restored controls pass. This
repairs source qualification consumers and does not accept any actual-runtime or CI row.


T019d adds the IMG-16 publication-credential boundary: `prepare.sh` builds and witnesses the
container in the credentialless read-only `verify` job (`GITHUB_TOKEN`/`GH_TOKEN` rejected before
execution), packs public assets and `image-layout-<tag>.tar.gz` under `HANDOFF.sha256`, and
`publish.sh` verifies handoff integrity + OCI-to-tarball config binding BEFORE logging in or
publishing by verified digest via pinned ORAS 1.3.0 (`oras.sh` + `oras.pins`) with registry
digest readback. `publish_runtime_test.sh` tests positive paths, tamper refusals, resolve mismatch,
and ORAS checksum verification. These controls do not constitute hosted CI or live publication.
