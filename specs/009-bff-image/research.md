# Research: BFF container image

## R1 Probe, read-only, outside the repository (2026-11-01)

Scratch module `m8-draft/oci-probe`, Bazel 9.3.0, rules_go 0.64.2, Go 1.27.2, `BAZEL_DO_NOT_DETECT_CPP_TOOLCHAIN=1`, as in this repository.

| Attempt                                               | Result                                                                                                                       |
| ----------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| rules_oci 2.3.0 with tar.bzl 0.7.0 `tar` and `mutate` | Failed analysis: `mutate` needs `gawk`, built from source, which needs a C++ toolchain. Kept as `probe-mutate-needs-cpp.log` |
| The same with a declared `mtree` line and no `mutate` | Built. One layer, one file, uid 65532, mtime 0, config `created` 1970, linux/amd64                                           |

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

## R7 Current rules_img and single-image serving amendment

The current selected builder is [rules_img v0.3.26](https://github.com/bazel-contrib/rules_img/releases/tag/v0.3.26)
as a candidate exact Bzlmod pin, not a compatibility result. Its tagged
[MODULE.bazel](https://github.com/bazel-contrib/rules_img/blob/v0.3.26/MODULE.bazel) declares
version 0.3.26 and a prebuilt image-tool toolchain. Its root-only development Go toolchain and
integration matrix do not establish compatibility in this consumer's Bazel 9.3.0/Go 1.27.4
graph. T015 must resolve and build the real consumer before this pin is accepted.

The tagged [quick start](https://github.com/bazel-contrib/rules_img/blob/v0.3.26/README.md#quick-start)
uses separate image, layer, load and push modules. No `img/defs.bzl` API is assumed. The load/push
interfaces and OCI-layout output group must be read at this pin and migrated together; a builder
name replacement alone cannot qualify existing witnesses or publishers. No base image or new
image feature is added without a demonstrated runtime need and provenance entry. Build-time push
remains disabled; credentialless acceptance must perform no registry publication.

The [Cloud Run container contract](https://docs.cloud.google.com/run/docs/container-contract#port)
requires the ingress process to listen on the injected `PORT` on every interface. The current
source already has production OIDC and listener configuration; T016–T018 must retain their
refusal and auth boundaries while serving the built UI and generated Connect path from the same
actual image. A source checkout, host-mounted assets, an extracted binary or a second build
cannot substitute for that runtime witness. Existing external signed local issuer fixtures
remain test infrastructure outside the production image.

Historical R1–R6, old runtime evidence and reproducibility limits remain recorded. The selected
amendment does not inherit their acceptance. T019 reconciles current docs, CI and publisher
consumers, including the historical separate Pages frontend in spec012; no deployment action or
shared template runtime is authorized by this source change.

### T015 consumer probe result

The root module now resolves rules_img 0.3.26 with the retained Bazel 9.3.0/Go 1.27.4 graph.
The actual production image, loader tarball and publisher executable build. The unchanged,
uncached image content/static/refusal/corruption test passes. The layout, loader tarball and
credentialless publisher's documented local OCI sink hold the same manifest bytes. The public
`image` target remains a complete OCI layout through a filegroup selecting `oci_layout`;
`image_tarball` retains its executable and `tarball` output group. The publisher's CLI now
splits registry and repository, as this pin requires. Build-time push is disabled, and exported
layout blobs are embedded rather than linked outside the output tree.

The first analysis caught an unsupported per-target `push_at_build_time` attribute on
`image_manifest`; the pin exposes that control as a global setting for this rule. The source
uses that global disabled setting and the supported per-publisher disabled attribute. The
normal namespace-sandbox attempt stopped because this host cannot create its user namespace.
The successful local probe explicitly uses the repository's `weak-isolation` profile; it is
not full-isolation or hosted qualification. No registry publication or container execution
occurred. The image at this increment still holds only the server; T016–T021 must deliver
and qualify the required UI, same-origin journey, real container and required CI evidence.

Pinned interfaces: [manifest output groups](https://github.com/bazel-contrib/rules_img/blob/v0.3.26/img/private/manifest.bzl),
[loader tarball](https://github.com/bazel-contrib/rules_img/blob/v0.3.26/img/private/load.bzl),
[offline sink and split CLI](https://github.com/bazel-contrib/rules_img/blob/v0.3.26/img_tool/cmd/deploy/deploy.go).

### T016/T017 serving and packaged UI source witnesses

The production composition now serves its built UI alongside the generated Connect routes.
`--web-root` / `BFF_WEB_ROOT` selects the asset directory (default `/web`).
`--signin-client-id` / `BFF_SIGNIN_CLIENT_ID` supplies only the public browser client identifier;
when present, config.json combines it with the configured verifier issuer. No sign-in metadata
is invented when it is absent. The RPC base URL is relative to the same origin. Configuration
responses are not cached; missing assets, directory listing and symlink escape return 404.
RPC authentication, verifier refusal, listener bounds and address precedence remain intact.

The rules_img layer contains exactly the static server and production `web/index.html` and
`web/main.js`, with declared non-root file metadata and epoch timestamps. It contains no
Bazel runfiles, source checkout, Node runtime, development identity or signing key.
A real browser source witness signs in through the existing external local issuer and calls
the generated client on the page origin: success and typed owner-isolation failure, plus
missing bearer and wrong audience rejection. A non-default `PORT` controls the actual
all-interface listener, with no overriding address flag. The test is enrolled in the e2e leg.

The first source browser setup omitted required telemetry inputs and was refused. With the
existing non-exporting telemetry fixture it started, then exposed a second fixture issue:
Bazel's sandbox stages tree input files as links outside the asset root. Native lstat readback
confirmed this, and the production OpenInRoot boundary correctly refused them. The fixture
now copies only the declared built bytes into owned regular files, with bounded process
shutdown and owned temporary-directory cleanup. Production confinement was not relaxed.
The telemetry fixture's synthetic version is not a release identity or export acceptance.

These local witnesses use the explicit weak-isolation profile. The source/browser test is
not a container run. The author account cannot access its local Docker socket; T018 must
obtain the actual same-image runtime and causal negative controls through an authorized
runtime path, and T019 must make them required in CI without weakening existing checks.
System trust roots for a non-loopback HTTPS issuer remain a runtime-content decision to
resolve before the optional hosted image can claim that capability; the local issuer uses
the existing explicitly permitted loopback HTTP path. No cloud, registry or deployment
operation follows from this increment.

### T018 identity binding preparation (runtime execution pending)

The container witness must bind the loaded runtime configuration to the qualified image rather
than trust a mutable tag. A source helper now verifies the built OCI descriptor closure and
its linux/amd64, numeric non-root, entrypoint and root-filesystem identity, returning the
manifest/configuration digests. Nine source tests include deterministic generated opaque
layer bytes and digest-valid wrong-user/platform/entrypoint/filesystem controls, corrupted
blobs, multiple manifests, malformed descriptor sizes and path-shaped digests. Their synthetic
fixtures test identity parsing only; they are not images or container witnesses. The helper
also read the actual built production layout successfully. Existing complexity and lint limits
remain enforced. Docker execution, artifact loading/container readback, real image journeys
and all T018 runtime negative controls remain pending.

### T018/T019 runtime witness source (actual container qualification pending)

The canonical `tools/checks/image_runtime.sh` command runs a test-only Playwright binary outside
Bazel's test namespace on the authorized disposable Linux CI host. It loads the actual rules_img
archive after checking its embedded configuration against the closed source OCI layout, records
archive/configuration/manifest/root-filesystem identities, then inspects the loaded immutable
configuration ID. Each container runs that ID with the image's own non-root entrypoint, packaged
assets, injected `PORT`, a read-only root, all capabilities dropped and no new privileges. Host
binds and volumes are refused by readback. The external local signed issuer and a local discard
OTLP receiver remain outside the image; that receiver permits normal drain but proves no collector
export acceptance. Docker receives a private empty configuration and a closed environment, uses
only the local Unix socket, and performs no pulls, pushes, registry login or daemon/access changes.
Cleanup is confined to each exact ownership-checked terminal container and the owned private
configuration directory. No image/cache pruning or unrelated resource cleanup is performed.

The shared source/container browser journey checks generated-client success, typed owner
isolation, explicit missing-bearer HTTP 401 and wrong-audience, wrong-issuer and invalid-signature
401 responses. Actual container controls must additionally observe an empty test-only tmpfs hiding
`/web` (404 while authenticated RPC still succeeds), an overriding address binding the wrong port,
and a planted one-nanosecond RPC deadline causing generated-client 504/deadline failure, each
beside the restored same-artifact positive. A missing-verifier container must terminate with the
exact production refusal, exit 1 and no listener. A separate production-source HTTP probe observed
20/20 deadline failures between normal 200 controls; it is not browser or container acceptance.
Fourteen readback-boundary tests reject substituted IDs, root execution, host mounts, foreign
ownership, writable roots, changed commands and security settings. Those native-shaped fixtures
are explicitly unit evidence, not proof that Docker ran.

The e2e CI leg requires this command after successful source verification and receipt export,
placing its own runtime evidence beneath that owned artifact directory. Source failure and export
failure retain priority; runtime failure cannot leave the CI leg green. The closed CI policy rejects
removed, conditional or tolerated runtime commands, and the actual workflow block is executed with
transport-only refusal stubs. The existing three source legs, aggregate, credentialless permissions
and protected context remain intact. The author host's local Docker socket remains unavailable;
actual image startup, browser controls, terminal readbacks and CI execution are still required.
HTTPS trust roots, final documentation reconciliation, unchanged full gates, three exact reviews,
protected delivery/main and distinct FUTURE remain open. This source increment accepts no full
amendment row and transfers no historical image approval.

Primary contracts: [Docker image load](https://docs.docker.com/reference/cli/docker/image/load/),
[Docker container options](https://docs.docker.com/reference/cli/docker/container/run/),
[Linux host networking](https://docs.docker.com/engine/network/drivers/host/) and
[Playwright worker fixtures](https://playwright.dev/docs/test-fixtures#worker-scoped-fixtures).

### T019a minimal HTTPS trust decision

The production scratch image has no filesystem CA bundle. Pin Go's binary-only
`golang.org/x/crypto/x509roots/fallback` module to
`v0.0.0-20261115193255-739070949446` (crypto commit
`739070949446de4a1e3d676e19b1bfa16655a2f4`). It embeds NSS public roots and registers them
through crypto/x509.SetFallbackRoots when the OS does not provide roots. Retain its upstream
constraints and default TLS verification; do not add a CA file, base image, provider, test key,
insecure TLS flag or trust override. The source unit target must make both system certificate
locations unavailable before process initialization, then verify a public trust anchor using
default roots and refuse a real loopback HTTPS server with an untrusted certificate. These
are trust-content/source witnesses, not configured live issuer or container acceptance.

Primary API: https://pkg.go.dev/golang.org/x/crypto/x509roots/fallback and
https://pkg.go.dev/crypto/x509#SetFallbackRoots . The exact module has Go minimum 1.26.0,
no external module requirements and the Go BSD-3-Clause license. Keep the pin current through
the existing dependency/vulnerability process; an embedded root snapshot does not update itself.
