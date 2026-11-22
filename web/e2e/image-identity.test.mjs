// Identity/parser witnesses only. Synthetic opaque layer bytes do not prove image behavior.
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, mkdir, writeFile, rm, readFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import fc from "fast-check";

import { readImageIdentity } from "./image-identity.mjs";

const sha = (bytes) => "sha256:" + createHash("sha256").update(bytes).digest("hex");
const media = "application/vnd.oci.image.";
async function fixture(t, patch = {}, bytes = Buffer.from("opaque identity fixture")) {
  const root = await mkdtemp(path.join(process.env.TEST_TMPDIR ?? os.tmpdir(), "image-identity-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  await mkdir(path.join(root, "blobs", "sha256"), { recursive: true });
  async function blob(value, type) {
    const data = Buffer.isBuffer(value) ? value : Buffer.from(JSON.stringify(value));
    const digest = sha(data);
    await writeFile(path.join(root, "blobs", "sha256", digest.slice(7)), data);
    return { mediaType: media + type, digest, size: data.length };
  }
  const layer = await blob(bytes, "layer.v1.tar");
  const config = await blob(
    {
      architecture: "amd64",
      os: "linux",
      config: { User: "65532:65532", Entrypoint: ["/bff"] },
      rootfs: { type: "layers", diff_ids: [layer.digest] },
      ...patch,
    },
    "config.v1+json",
  );
  const manifest = await blob({ schemaVersion: 2, config, layers: [layer] }, "manifest.v1+json");
  await writeFile(
    path.join(root, "index.json"),
    JSON.stringify({ schemaVersion: 2, manifests: [manifest] }),
  );
  await writeFile(path.join(root, "oci-layout"), JSON.stringify({ imageLayoutVersion: "1.0.0" }));
  return { root, layer, config, manifest };
}

test("closed OCI identities bind opaque layer bytes by digest", async (t) => {
  await fc.assert(
    fc.asyncProperty(fc.uint8Array({ maxLength: 512 }), async (bytes) => {
      const f = await fixture(t, {}, Buffer.from(bytes));
      const result = await readImageIdentity(f.root);
      assert.deepEqual(result, {
        manifestDigest: f.manifest.digest,
        configDigest: f.config.digest,
        diffIDs: [f.layer.digest],
      });
    }),
    { seed: 914021, numRuns: 30 },
  );
});

test("a changed blob cannot retain its claimed identity", async (t) => {
  const f = await fixture(t);
  await writeFile(path.join(f.root, "blobs", "sha256", f.layer.digest.slice(7)), "corrupted");
  await assert.rejects(readImageIdentity(f.root), /image identity: blob digest or size/);
});

for (const [name, patch] of [
  ["root user", { config: { User: "0", Entrypoint: ["/bff"] } }],
  ["foreign architecture", { architecture: "arm64" }],
  ["substituted entrypoint", { config: { User: "65532:65532", Entrypoint: ["/other"] } }],
  ["unbound filesystem", { rootfs: { type: "layers", diff_ids: ["sha256:" + "0".repeat(64)] } }],
]) {
  test(`a digest-valid ${name} is refused`, async (t) => {
    const f = await fixture(t, patch);
    await assert.rejects(readImageIdentity(f.root), /image identity: runtime contract/);
  });
}

test("multiple manifests are refused", async (t) => {
  const f = await fixture(t);
  await writeFile(
    path.join(f.root, "index.json"),
    JSON.stringify({ schemaVersion: 2, manifests: [f.manifest, f.manifest] }),
  );
  await assert.rejects(readImageIdentity(f.root), /image identity: one manifest/);
});

test("descriptor paths cannot escape the blob directory", async (t) => {
  const f = await fixture(t);
  await fc.assert(
    fc.asyncProperty(
      fc.constantFrom(
        "../private",
        "sha256:../private",
        "sha512:" + "a".repeat(64),
        "sha256:" + "a".repeat(63),
      ),
      async (digest) => {
        await writeFile(
          path.join(f.root, "index.json"),
          JSON.stringify({ schemaVersion: 2, manifests: [{ ...f.manifest, digest }] }),
        );
        await assert.rejects(readImageIdentity(f.root), /image identity: invalid descriptor/);
      },
    ),
    { seed: 914022, numRuns: 12 },
  );
});

test("malformed descriptor size is refused", async (t) => {
  const f = await fixture(t);
  const index = JSON.parse(await readFile(path.join(f.root, "index.json"), "utf8"));
  index.manifests[0].size = -1;
  await writeFile(path.join(f.root, "index.json"), JSON.stringify(index));
  await assert.rejects(readImageIdentity(f.root), /image identity: invalid descriptor/);
});
