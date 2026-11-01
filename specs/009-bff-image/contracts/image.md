# Contract: BFF image

| Property | Value |
| --- | --- |
| Target | `//bff/cmd/bff:image`, an OCI layout |
| Builder | Bazel with rules_oci. Nothing else builds this image |
| Platform | linux/amd64 |
| Base | None |
| Layers | One, holding `/bff` only |
| User | `65532:65532`, numeric |
| Entrypoint | `["/bff"]` |
| Env, Cmd, ExposedPorts | None. A listen address is the server's own flag |
| Timestamps | Epoch |
| Identity | The manifest's content digest, computed from the bytes, of a plain `tools/bootstrap/bazel.sh build`. The `image-release` check builds that image, runs the witness on it and reports the digest as a measurement in the receipt. A runtime run binds that digest. An image built by the verifier's `bazel coverage` invocation holds the same uninstrumented server but a different digest, because Bazel names its transitioned output directories by their difference from the command's top-level configuration, and the Go binary records those directories in its generated sources' paths. That image witnesses content and behavior, and is never the release artifact |
| Linking | No `PT_INTERP`, no `DT_NEEDED` |

**Startup.** As shipped, with no verifier assigned, the process exits 1 and writes exactly this to stderr:

```text
bff: no verifier is configured; assign newVerifier in bff/cmd/bff/verifier.go
```

A healthy production start is unsupported until an adopter assigns a real verifier.

**An image or a tarball is not startup.** Building either proves nothing about serving. The acceptance result is the
process's observed behavior: today, this refusal. A profile that claims a healthy serving container must show real
startup and real HTTP journeys against the configured artifact before it claims readiness.

**A hosted pipeline** may run the Bazel target and publish the resulting digest. It does not rebuild the image.
