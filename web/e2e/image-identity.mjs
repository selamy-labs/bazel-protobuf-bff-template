// Bind a built OCI layout to the immutable runtime configuration ID (spec009 T018).
// Image content/ELF acceptance remains the canonical image_test; this checks descriptor closure.
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { isDeepStrictEqual } from "node:util";

const media = "application/vnd.oci.image.";
async function blob(root, descriptor, type) {
  if (
    !descriptor ||
    descriptor.mediaType !== media + type ||
    !/^sha256:[a-f0-9]{64}$/.test(descriptor.digest) ||
    !Number.isSafeInteger(descriptor.size) ||
    descriptor.size < 0
  ) {
    throw new Error("image identity: invalid descriptor");
  }
  const data = await readFile(path.join(root, "blobs", "sha256", descriptor.digest.slice(7)));
  const digest = "sha256:" + createHash("sha256").update(data).digest("hex");
  if (digest !== descriptor.digest || data.length !== descriptor.size)
    throw new Error("image identity: blob digest or size");
  return data;
}

export async function readImageIdentity(root) {
  const index = JSON.parse(await readFile(path.join(root, "index.json"), "utf8"));
  if (index.schemaVersion !== 2 || !Array.isArray(index.manifests) || index.manifests.length !== 1)
    throw new Error("image identity: one manifest required");
  const descriptor = index.manifests[0];
  const manifest = JSON.parse(await blob(root, descriptor, "manifest.v1+json"));
  const config = JSON.parse(await blob(root, manifest.config, "config.v1+json"));
  if (!Array.isArray(manifest.layers) || manifest.layers.length === 0)
    throw new Error("image identity: runtime contract");
  for (const layer of manifest.layers) await blob(root, layer, "layer.v1.tar");
  const diffIDs = manifest.layers.map((layer) => layer.digest);
  const actual = {
    os: config.os,
    architecture: config.architecture,
    user: config.config?.User,
    entrypoint: config.config?.Entrypoint,
    rootfs: config.rootfs,
  };
  const expected = {
    os: "linux",
    architecture: "amd64",
    user: "65532:65532",
    entrypoint: ["/bff"],
    rootfs: { type: "layers", diff_ids: diffIDs },
  };
  if (!isDeepStrictEqual(actual, expected)) throw new Error("image identity: runtime contract");
  return { manifestDigest: descriptor.digest, configDigest: manifest.config.digest, diffIDs };
}
