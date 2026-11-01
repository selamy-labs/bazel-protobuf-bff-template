# Research: BFF container image

## R1 Probe, read-only, outside the repository (2026-11-01)

Scratch module `m8-draft/oci-probe`, Bazel 9.3.0, rules_go 0.64.2, Go 1.27.2, `BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=1`, as in this repository.

| Attempt | Result |
| --- | --- |
| rules_oci 2.3.0 with tar.bzl 0.7.0 `tar` and `mutate` | Failed analysis: `mutate` needs `gawk`, built from source, which needs a C++ toolchain. Kept as `probe-mutate-needs-cpp.log` |
| The same with a declared `mtree` line and no `mutate` | Built. One layer, one file, uid 65532, mtime 0, config `created` 1970, linux/amd64 |

**Decision:** declare the layer's mtree directly. This repository has no C++ toolchain, on purpose, so upstream's `mutate` example does not apply.

Not proven by the probe: a run on the compiler-free qualified host, and resolution beside this repository's rules_js. rules_oci's module graph includes bazel_lib 3.0.0, so the lock change at landing is its own check.

## R2 Version

The registry's newest rules_oci is 2.3.0, with nothing yanked, read 2026-11-01. Upstream's README calls the project "stable in maintenance mode", and states no Bazel version, so the pin rests on the probe and not on a statement of support.

## R3 No base image

`base` is optional, and the server is pure Go with cgo off. With no base there is nothing to pull, no digest to track and no base license, and FR-007 stays offline. The cost falls on a later adopter whose verifier needs CA roots, and that adopter adds them. A base would be a decision of its own, with a digest and a license.

## R4 Witnesses

A Go test reads the OCI layout with the standard library, following `index.json` to the manifest, config and layer, and checks every digest and size against the bytes. It adds no tool and no runtime. `debug/elf` on the packaged binary shows no `PT_INTERP` and no `DT_NEEDED`, because a missing interpreter alone does not rule out dynamic dependencies. It then runs that entrypoint, and compares exit and stderr.

The startup claim has two layers. The extracted entrypoint runs in the verifier. A real runtime is separate evidence: the authoring account has no container daemon access, so that run belongs to an authorized host, through `oci_load`'s tarball, and it binds the same content digest. Neither is presented as the other.

## R5 Repeatability

A test in one output base can only see one build. A script outside the verifier builds in two clones at different paths, with separate output bases, and compares manifest digests.

## R6 Coverage mode, found by the first full verifier (2026-11-01)

The full verifier runs `bazel coverage`, and its instrumentation filter covers `//bff`. The first full run at
`29d0d6c` failed: the image held an instrumented server, whose stderr carried coverage warnings before the
refusal. A transition on the packaged binary resets `collect_code_coverage`, so the image holds an uninstrumented
server under either command. The digest still differs between the two commands, by one cause measured on the
binaries: the generated `.pb.go` sources are recorded with the output directory of the configuration that made
them, `bazel-out/k8-fastbuild/bin` in a plain build and a transitioned directory under coverage. Same size, no
shim, and no other differing string. A second measurement, after resetting `instrumentation_filter` in the same transition: the two transitioned
configurations became option-identical, with the same configuration ids under both commands, and the output
directories still differed. Bazel's `DIFF_AGAINST_DYNAMIC_BASELINE` naming scheme derives a transitioned
directory's name from its difference to the command's own top-level configuration, which differs between
`build` and `coverage`. No option normalization closes that, and changing the naming scheme for the whole
repository is not a packaging decision. So the transition keeps the server uninstrumented under either command,
identity is the plain build's digest, and the `image-release` check builds and witnesses that plain image after
the verifier's own invocation and reports its digest. Coverage of the server's own code is unchanged: it comes
from the tests, not the image.

## Alternatives rejected

- A Dockerfile or Cloud Build image step: a second builder, and the conflict this feature removes.
- A distroless base: more than the server needs today.
- Packaging `bff-dev` or a stub verifier for a green start: rejected by decision. It would claim a health the product does not have.

## R-adoption Platform indexes and signing at rules_oci 2.3.0

The pinned rules_oci 2.3.0 documents platform-driven image indexes without the module's usual compatibility
guarantee and signing as preview. This image declares one platform, `linux/amd64`, has no base, and is identified by
the content digest the build produces; `image_index` and signing stay out of the supported profile until each is
qualified locally at the pinned version against the same digest and the same refusal. Independent digests from two
clean builds at the final packaging revision remain the reproducibility witness (spec 009 FR-006).
