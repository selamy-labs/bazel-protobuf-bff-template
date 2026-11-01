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
| Identity | The manifest's content digest, computed from the bytes |
| Linking | No `PT_INTERP`, no `DT_NEEDED` |

**Startup.** As shipped, with no verifier assigned, the process exits 1 and writes exactly this to stderr:

```text
bff: no verifier is configured; assign newVerifier in bff/cmd/bff/verifier.go
```

A healthy production start is unsupported until an adopter assigns a real verifier.

**A hosted pipeline** may run the Bazel target and publish the resulting digest. It does not rebuild the image.
