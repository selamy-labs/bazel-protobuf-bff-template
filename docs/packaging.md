# Browser-app image packaging

The current browser-app candidate has one rules_img 0.3.26 image for linux/amd64. Its numeric
non-root static Go entrypoint serves the built production UI and generated Connect handlers on
one origin, reads `PORT`, and needs no host-mounted assets or Bazel runfiles. Public fallback
TLS roots are pinned and embedded in the executable; no CA file, signing key, Node runtime or
new base image is added. An adopter owns its selected source and runtime; the template is not
a shared BFF service.

The production image requires issuer/audience/JWKS, exact allowed-origin and telemetry inputs.
`BFF_SIGNIN_CLIENT_ID` is the public browser client identifier. It supplies no credential or
identity-provider default. Missing verifier inputs refuse startup; bearer verification remains
required for RPC. The example memory store does not prove managed persistence or live readiness.

Build and inspect the public layout with the pinned wrapper:

```sh
tools/bootstrap/bazel.sh build //bff/cmd/bff:image
tools/bootstrap/bazel.sh cquery --output=files //bff/cmd/bff:image
tools/bootstrap/bazel.sh test //bff/cmd/bff:image_test
```

On an authorized Linux Docker host with a clean exact checkout, the canonical actual-container
witness is:

```sh
runtime_outputs="$(mktemp -d)"
BFF_IMAGE_RUNTIME_OUTPUTS="$runtime_outputs" tools/checks/image_runtime.sh
```

The command refuses any tracked or untracked dirty checkout, binds both `HEAD` and `HEAD^{tree}`,
defaults to `unix:///var/run/docker.sock` (or an explicit local-only `unix://` socket via
`BFF_IMAGE_DOCKER_HOST`, refusing remote or traversal endpoints), checks Docker Engine API
compatibility (`>= 1.41`), loads the actual built archive, checks its configuration against the
closed OCI layout, runs the immutable configuration ID, verifies owned-container retirement
(`runtime-terminal.json` with `ownedContainersRemaining: 0`), and retains archive/manifest/config/rootfs
hashes, Docker version, exact container inspection, browser results and terminal readback. Seven tests
cover actual same-origin generated-client success/typed owner isolation, bearer negatives,
missing assets, wrong port, broken RPC and missing-verifier refusal, with restored controls.
The issuer and discard telemetry receiver are external test fixtures; the latter proves no
collector/export acceptance. No registry access or live provider operation is performed.
Offline contract and script refusal controls (`//web/e2e:runtime_contract_test`,
`//:image_runtime_test`) verify daemon-host validation, API compatibility, source/tree/digest binding,
and teardown refusal without claiming a live container ran.

The existing credentialless e2e CI leg requires this command after successful source checks and
receipt export. Its own evidence is placed beneath that owned artifact directory. A source-only
verifier result cannot certify a container. The trusted tag publisher also requires the actual
runtime command before registry login, compares its accepted manifest digest with the plain
image witness, and refuses failure, missing identity or a substituted digest. It attaches the
runtime proof and dependency notice with the release hashes. The optional Cloud Build profile
continues to promote/deploy the published digest without rebuilding and retains exact saved-plan
and production-approval gates. This source work authorizes no publication or deployment execution.

Current qualification is incomplete. Local source/browser, image-content, embedded-root and
publication-guard witnesses pass, but the author host's Docker socket admission was refused before
load/create/run. Actual container CI, full exact-source review and protected delivery remain open.
The legacy spec012 Pages profile and its historical evidence are preserved. The selected typed
image profile is `deploy/gcp/inputs/same-origin.inputs.json`: root UI, RPC equal to the exact
runtime origin, and the existing verifier audience passed as `BFF_SIGNIN_CLIENT_ID`. The legacy
`example.inputs.json` remains a Pages fixture. Both profiles and the client binding have offline
source witnesses; those do not establish live configuration.
No live issuer, registry release, deployment, DNS or production state is claimed.
