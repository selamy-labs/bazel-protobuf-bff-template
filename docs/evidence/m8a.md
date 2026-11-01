# Evidence: M8a BFF container image

A claim holds only for the revision and label beside it. All of this is author-run on the authoring host,
labeled `weakened`, and nothing here is independent yet.

## Identities

| Item | Value |
| --- | --- |
| Image source | `4fe7355`: the targets, the witness, the uninstrumented transition and the required dynamic control. The production image's content is unchanged from `779a36e` |
| Manifest digest, computed from the bytes | `sha256:682d4fae470d8dd11d741b26ea7eb54a7e98f4b2e2fe4b90c41605aeb27c2189`, 477 bytes |
| Config digest | `sha256:80caa02a611d56bf7f54626fd7b74dad0d2b22e632bc14ea8dbbaf511cfe2ad8` |
| Layer digest | `sha256:c2c9554f5ebc07ddfbd8387890d14f7b238f18f7b4219505a047a95969d9945d`, 14362624 bytes, one file `./bff` |
| Loadable tarball | `tools/bootstrap/bazel.sh build --output_groups=+tarball //bff/cmd/bff:image_tarball`, at `bazel-bin/bff/cmd/bff/image_tarball/tarball.tar`, sha256 `e24bc8f2139d63235a14f553f5cf061072f19c61aebbe3c80483a051cf0d84e5`, tag `bff-template/bff:local`. Its manifest names the same config and layer digests as the layout |

## Claims kept separate

| Claim | State |
| --- | --- |
| Witness before the image | `//bff/cmd/bff:image_test` failed at `a782493` with no layout to read, five tests |
| Witness with the image | Passed at `779a36e`: descriptor closure checked from content, linux/amd64, user `65532:65532`, entrypoint `/bff`, one file, no `PT_INTERP`, no `PT_DYNAMIC`, no `DT_NEEDED`, and the packaged entrypoint exits 1 with the exact refusal and prints nothing on stdout. The extracted entrypoint runs as the test's own user, not as 65532 |
| Named defects | Each failed the witness, with the control passing before and after: uid 0, a second file, a wrong entrypoint, a named user, a blob that does not match its digest or size. Packaging `bff-dev` is refused earlier, by Bazel's analysis, because that binary is `testonly` |
| Dynamic binary, packaged | In a private clone of `4fe7355`, the host's shell packaged as `/bff`: the image built, and the static witness rejected it naming `PT_INTERP`, `PT_DYNAMIC` and `libc.so.6`. The restored control passed. The witness's own control also fails, rather than skips, on a host whose `/bin/sh` cannot be read |
| Repeatability | Two clean clones of `779a36e`, two paths, two output bases, two `HOME`s: one manifest digest, `682d4fae…2189`, hashed from each layout's bytes |
| Real runtime, independent | The author-built artifact, manifest `682d4fae…2189`, loaded from the tarball `e24bc8f2…84e5` into a Docker engine on another host: image user and entrypoint unchanged, no network, read-only root, no capabilities, 1 CPU, 256 MiB. It exited 1 in 100 ms with the exact refusal on stderr and nothing on stdout. Docker's own image ID, `sha256:0554f08a…`, is its schema2 re-serialization of the manifest; the config bytes `80caa02a…` and layer bytes `c2c9554f…` were exported back and matched the OCI closure, so that run binds this artifact's content and not Docker's ID. It is execution of the author's artifact, not an independent rebuild, and no service was healthy |
| Verifier, `29d0d6c` | **Failed**, exit 1, 349 s: under `bazel coverage` the image held an instrumented server, and `image_test` saw coverage warnings before the refusal. Earned nothing. The repair is `4fe7355` |
| Coverage mode after the repair | `image_test` passes under both `bazel test` and `bazel coverage`. The coverage-mode image's digest differs from the plain one by its embedded generated-source directory only, measured on the binaries. With `instrumentation_filter` also reset, the two transitioned configurations are option-identical, same ids under both commands, and the directories still differ by Bazel's naming scheme. So the plain build is the release artifact, and the `image-release` check witnesses it and reports its digest in the receipt |
| Verifier, `eae69a9` | Passed, 27 of 27 checks in 351 s, clean tree. `image-release` reported `image sha256:682d4fae470d8dd11d741b26ea7eb54a7e98f4b2e2fe4b90c41605aeb27c2189` in the receipt |
| Verifier, `05d2545` | Passed, 27 of 27 checks in 345 s, clean tree, after the witness replay became uncached and read by the pinned yq. The image is unchanged |
| Repeatability, `eae69a9` | Two clean clones, two paths, two output bases: one manifest digest, `682d4fae…2189`, equal to the `779a36e` artifact. The packaging change kept the production content |

## Not claimed

- A healthy production start: the server refuses to start without a verifier, and the image shows exactly that.
- Anything about macOS or linux/arm64: the targets are incompatible there, and `test-inventory` on such a host is unverified.
- The license of the `platforms` module: its fetched source carries no LICENSE file, so it is an unresolved inventory finding.

## Results that were not credited

- The full verifier at `29d0d6c` failed, above.
- The first packaged-dynamic control mutated nothing: its patterns named the pre-transition target, so no image changed and it was a setup failure. The rerun asserts the mutation applied.
- The first defect run expected the witness to fail on a packaged `bff-dev`; analysis refused it first. The expectation was corrected and the run repeated from the control.
- The first tarball handoff named the layer tar as the loadable tarball, from a file name alone. `oci_load` writes `tarball.tar` only for the `tarball` output group. The record above is from that output's content.
- `9d3bdd8` was committed past a failing `format-starlark` result, because the gate's exit was not chained to the commit. `779a36e` corrects the file, and later commits chain the gate.
