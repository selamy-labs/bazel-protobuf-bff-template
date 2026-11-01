# Evidence: M8a BFF container image

A claim holds only for the revision and label beside it. All of this is author-run on the authoring host,
labeled `weakened`, and nothing here is independent yet.

## Identities

| Item | Value |
| --- | --- |
| Image source | `779a36e`: the targets, the witness and their formatting. No file under `bff/`, `MODULE.bazel` or the lock changed after it through this record's own commit |
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
| Dynamic binary | The static check sees the host's `/bin/sh` as dynamic, by both its interpreter and its needed entries. No dynamic Go build was produced, because this repository has no C++ toolchain |
| Repeatability | Two clean clones of `779a36e`, two paths, two output bases, two `HOME`s: one manifest digest, `682d4fae…2189`, hashed from each layout's bytes |
| Real runtime | Not run here. The authoring account has no container daemon access. An independent run binds the same content digest |
| Verifier | See the record below for the candidate's full result |

## Not claimed

- A healthy production start: the server refuses to start without a verifier, and the image shows exactly that.
- Anything about macOS or linux/arm64: the targets are incompatible there, and `test-inventory` on such a host is unverified.
- The license of the `platforms` module: its fetched source carries no LICENSE file, so it is an unresolved inventory finding.

## Results that were not credited

- The first defect run expected the witness to fail on a packaged `bff-dev`; analysis refused it first. The expectation was corrected and the run repeated from the control.
- The first tarball handoff named the layer tar as the loadable tarball, from a file name alone. `oci_load` writes `tarball.tar` only for the `tarball` output group. The record above is from that output's content.
- `9d3bdd8` was committed past a failing `format-starlark` result, because the gate's exit was not chained to the commit. `779a36e` corrects the file, and later commits chain the gate.
