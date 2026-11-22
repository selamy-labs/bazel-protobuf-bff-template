# Quickstart: BFF image

## Current migration status

The public image targets now use pinned rules_img 0.3.26 and package the production server
with its built UI at `/web`. Local content and same-origin source/browser checks pass. The
actual same-image container, required CI and final delivery qualification are pending under
T018–T021; historical refusal-only evidence does not qualify the new image.

The production composition requires its existing issuer, telemetry and allowed-origin inputs.
Set `PORT` for the listener and `BFF_SIGNIN_CLIENT_ID` for the public browser sign-in client;
config.json supplies a relative same-origin RPC URL. `BFF_WEB_ROOT` overrides the `/web`
asset directory for an explicitly owned source run; an image run needs no asset mount.
Optional publishing/deployment retains its separate operator gates.

## Preserved public build interfaces

Prerequisites: the supported profile, and the wrapper `tools/bootstrap/bazel.sh`. No container runtime is needed until the last step.

1. Build it, and find the layout:

   ```bash
   tools/bootstrap/bazel.sh build //bff/cmd/bff:image
   ```

   Find the actual layout path with `tools/bootstrap/bazel.sh cquery --output=files //bff/cmd/bff:image`; it holds what [contracts/image.md](contracts/image.md) states.

2. Run the witness:

   ```bash
   tools/bootstrap/bazel.sh test //bff/cmd/bff:image_test
   ```

   Expect a pass. It checks the image's content and runs the packaged entrypoint.

3. On an authorized Linux host with its local Unix Docker engine available, run the actual-image
   browser and causal controls from a clean exact checkout:

   ```bash
   runtime_outputs="$(mktemp -d)"
   BFF_IMAGE_RUNTIME_OUTPUTS="$runtime_outputs" tools/checks/image_runtime.sh
   ```

   The canonical command owns loading, immutable-ID binding, seven tests and exact container cleanup.
   It requires both the real serving journey and the verifier refusal; an extracted binary or tarball
   alone does not qualify it. See [current packaging status](../../docs/packaging.md). Actual runtime
   execution and final FEATURE/main/FUTURE acceptance remain pending.
