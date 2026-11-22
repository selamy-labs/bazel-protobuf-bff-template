// Native-readback contract controls only; these fixtures never claim a container ran.
import assert from "node:assert/strict";
import test from "node:test";
import { checkLoadedImage, checkContainer, checkReady } from "./runtime-contract.mjs";

const identity = {
  configDigest: "sha256:" + "a".repeat(64),
  diffIDs: ["sha256:" + "b".repeat(64)],
};
const owner = { name: "bff-image-runtime-fixture", label: "owned-fixture", cid: "c".repeat(64) };
function image() {
  return {
    Id: identity.configDigest,
    Os: "linux",
    Architecture: "amd64",
    Config: { User: "65532:65532", Entrypoint: ["/bff"], Cmd: null },
    RootFS: { Type: "layers", Layers: identity.diffIDs },
  };
}
function container() {
  return {
    Id: owner.cid,
    Name: "/" + owner.name,
    Image: identity.configDigest,
    Config: {
      User: "65532:65532",
      Entrypoint: ["/bff"],
      Cmd: null,
      Labels: { "bff-template.runtime": owner.label },
    },
    HostConfig: {
      NetworkMode: "host",
      ReadonlyRootfs: true,
      CapDrop: ["ALL"],
      SecurityOpt: ["no-new-privileges"],
      Binds: null,
      Tmpfs: null,
    },
    Mounts: [],
  };
}
test("exact loaded configuration and unmounted non-root container readbacks pass", () => {
  checkLoadedImage(image(), identity);
  checkContainer(container(), identity, owner, false);
});
for (const [name, mutate] of [
  [
    "different image ID",
    (v) => {
      v.Id = "sha256:" + "d".repeat(64);
    },
  ],
  [
    "different filesystem",
    (v) => {
      v.RootFS.Layers = [];
    },
  ],
  [
    "root user",
    (v) => {
      v.Config.User = "0";
    },
  ],
  [
    "overridden command",
    (v) => {
      v.Config.Cmd = ["--other"];
    },
  ],
])
  test(`loaded ${name} is refused`, () => {
    const v = image();
    mutate(v);
    assert.throws(() => checkLoadedImage(v, identity), /loaded image identity/);
  });
for (const [name, mutate] of [
  [
    "foreign ownership",
    (v) => {
      v.Config.Labels["bff-template.runtime"] = "other";
    },
  ],
  [
    "substituted image",
    (v) => {
      v.Image = "sha256:" + "d".repeat(64);
    },
  ],
  [
    "writable root",
    (v) => {
      v.HostConfig.ReadonlyRootfs = false;
    },
  ],
  [
    "host bind",
    (v) => {
      v.HostConfig.Binds = ["/host:/web"];
    },
  ],
  [
    "named volume",
    (v) => {
      v.Mounts = [{ Type: "volume", Destination: "/web" }];
    },
  ],
  [
    "ambient capabilities",
    (v) => {
      v.HostConfig.CapDrop = [];
    },
  ],
  [
    "unapproved tmpfs",
    (v) => {
      v.HostConfig.Tmpfs = { "/web": "ro" };
    },
  ],
])
  test(`container ${name} is refused`, () => {
    const v = container();
    mutate(v);
    assert.throws(() => checkContainer(v, identity, owner, false), /container runtime contract/);
  });
test("the missing-assets control permits only its declared empty tmpfs", () => {
  const v = container();
  v.HostConfig.Tmpfs = { "/web": "ro,noexec,nosuid,nodev,size=1m,mode=0555" };
  checkContainer(v, identity, owner, true);
  v.HostConfig.Tmpfs["/elsewhere"] = "ro";
  assert.throws(() => checkContainer(v, identity, owner, true), /container runtime contract/);
});
test("actual ready address must match PORT on all interfaces", () => {
  const v = { event: "ready", addr: "[::]:17421", mode: "production", identity: "verified" };
  checkReady(v, 17421);
  for (const addr of ["127.0.0.1:17421", "[::]:17422", "0.0.0.0:8080"]) {
    assert.throws(() => checkReady({ ...v, addr }, 17421), /PORT listener mismatch/);
  }
  assert.throws(() => checkReady({ ...v, identity: "test" }, 17421), /PORT listener mismatch/);
});
