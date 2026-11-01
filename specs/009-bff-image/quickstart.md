# Quickstart: BFF image

Prerequisites: the supported profile, and the wrapper `tools/bootstrap/bazel.sh`. No container runtime is needed until the last step.

1. Build it, and find the layout:

   ```bash
   tools/bootstrap/bazel.sh build //bff/cmd/bff:image
   ```

   Expect an OCI layout at `bazel-bin/bff/cmd/bff/image`, holding what [contracts/image.md](contracts/image.md) states.

2. Run the witness:

   ```bash
   tools/bootstrap/bazel.sh test //bff/cmd/bff:image_test
   ```

   Expect a pass. It checks the image's content and runs the packaged entrypoint.

3. On a host with a container runtime, load the tarball and start it:

   ```bash
   tools/bootstrap/bazel.sh build //bff/cmd/bff:image_tarball
   docker load --input bazel-bin/bff/cmd/bff/image_tarball/tarball.tar
   docker run --rm --network none <the loaded image ID>
   ```

   Expect exit 1 and the exact refusal in the contract. That is the supported result today.
