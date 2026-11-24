// Native-readback contract controls only; these fixtures never claim a container ran.
import assert from "node:assert/strict";
import test from "node:test";
import {
  checkContainer,
  checkDaemonVersion,
  checkLoadedImage,
  checkReady,
  checkRuntimeReceipt,
  reconcileOwnedContainers,
  resolveLocalDockerHost,
} from "./runtime-contract.mjs";

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

test("resolveLocalDockerHost accepts default and local unix sockets and refuses remote or traversal endpoints", () => {
  assert.equal(resolveLocalDockerHost(undefined), "unix:///var/run/docker.sock");
  assert.equal(resolveLocalDockerHost(""), "unix:///var/run/docker.sock");
  assert.equal(
    resolveLocalDockerHost("unix:///run/user/1000/docker.sock"),
    "unix:///run/user/1000/docker.sock",
  );
  for (const bad of [
    "tcp://127.0.0.1:2375",
    "http://127.0.0.1:2375",
    "ssh://root@example.invalid",
    "unix://relative.sock",
    "unix:///run/../var/run/docker.sock",
    "unix:///var/run/docker.sock extra",
  ]) {
    assert.throws(() => resolveLocalDockerHost(bad), /local unix:\/\/ socket/);
  }
});

test("checkDaemonVersion accepts supported Docker Engine APIs and refuses unavailable or old daemons", () => {
  assert.deepEqual(checkDaemonVersion({ Server: { Version: "29.1.3", ApiVersion: "1.52" } }), {
    serverVersion: "29.1.3",
    serverApiVersion: "1.52",
  });
  assert.deepEqual(checkDaemonVersion({ Server: { Version: "24.0.7", ApiVersion: "1.43" } }), {
    serverVersion: "24.0.7",
    serverApiVersion: "1.43",
  });
  for (const unavailable of [null, {}, { Server: {} }, { Server: { Version: "29.1.3" } }]) {
    assert.throws(() => checkDaemonVersion(unavailable), /daemon unavailable/);
  }
  assert.throws(
    () => checkDaemonVersion({ Server: { Version: "19.03.0", ApiVersion: "1.40" } }),
    /incompatible docker daemon API version/,
  );
});

test("checkContainer enforces run-ownership label when runId is declared", () => {
  const withRun = { ...owner, runId: "run-123" };
  const c = container();
  c.Config.Labels["bff-template.runtime-run"] = "run-123";
  checkContainer(c, identity, withRun, false);
  c.Config.Labels["bff-template.runtime-run"] = "foreign-run";
  assert.throws(() => checkContainer(c, identity, withRun, false), /container runtime contract/);
});

test("checkRuntimeReceipt binds exact revision, tree, digests, daemon compatibility, and zero remaining containers", () => {
  const expected = {
    revision: "1".repeat(40),
    sourceTree: "2".repeat(40),
    manifestDigest: "sha256:" + "a".repeat(64),
    configDigest: "sha256:" + "b".repeat(64),
  };
  const validReceipt = () => ({
    schemaVersion: 1,
    ...expected,
    sourceDirty: false,
    dockerHost: "unix:///var/run/docker.sock",
    serverVersion: "29.1.3",
    serverApiVersion: "1.52",
    ownedContainersRemaining: 0,
    teardownVerified: true,
  });
  checkRuntimeReceipt(validReceipt(), expected);
  for (const [label, mutate, pattern] of [
    ["mismatched revision", (r) => (r.revision = "9".repeat(40)), /runtime terminal receipt/],
    ["mismatched sourceTree", (r) => (r.sourceTree = "8".repeat(40)), /runtime terminal receipt/],
    ["dirty source", (r) => (r.sourceDirty = true), /runtime terminal receipt/],
    [
      "mismatched manifestDigest",
      (r) => (r.manifestDigest = "sha256:" + "f".repeat(64)),
      /runtime terminal receipt/,
    ],
    [
      "mismatched configDigest",
      (r) => (r.configDigest = "sha256:" + "e".repeat(64)),
      /runtime terminal receipt/,
    ],
    ["remaining container", (r) => (r.ownedContainersRemaining = 1), /runtime terminal receipt/],
    ["unverified teardown", (r) => (r.teardownVerified = false), /runtime terminal receipt/],
    ["remote dockerHost", (r) => (r.dockerHost = "tcp://127.0.0.1:2375"), /local unix:\/\/ socket/],
    ["old daemon API", (r) => (r.serverApiVersion = "1.39"), /incompatible docker daemon API/],
  ]) {
    const r = validReceipt();
    mutate(r);
    assert.throws(() => checkRuntimeReceipt(r, expected), pattern, `case ${label} must fail`);
  }
});

test("reconcileOwnedContainers deduplicates full IDs across tracked and lost-response containers", async () => {
  const runId = "run-lost-response";
  const trackedCid = "a".repeat(64);
  const lostCid = "b".repeat(64);
  const containers = new Map([
    [
      trackedCid,
      {
        Id: trackedCid,
        Config: { Labels: { "bff-template.runtime-run": runId } },
        State: { Running: false },
      },
    ],
    [
      lostCid,
      {
        Id: lostCid,
        Config: { Labels: { "bff-template.runtime-run": runId } },
        State: { Running: true },
      },
    ],
  ]);
  const calls = [];
  const docker = async (...args) => {
    calls.push(args);
    if (args[0] === "ps") {
      const full = args.includes("--no-trunc");
      return [...containers.keys()].map((id) => (full ? id : id.slice(0, 12))).join("\n");
    }
    if (args[0] === "container" && args[1] === "inspect") {
      const found = containers.get(args[2]);
      if (!found) throw new Error(`no such container: ${args[2]}`);
      return JSON.stringify([found]);
    }
    if (args[0] === "container" && args[1] === "stop") {
      const found = containers.get(args[3]);
      if (!found) throw new Error(`no such container: ${args[3]}`);
      found.State.Running = false;
      return args[3];
    }
    if (args[0] === "container" && args[1] === "rm") {
      if (!containers.delete(args[2])) throw new Error(`no such container: ${args[2]}`);
      return args[2];
    }
    throw new Error(`unexpected docker args: ${args.join(" ")}`);
  };

  // Tracked container also appears in daemon discovery alongside lost-response container `lostCid`.
  const owned = new Set([trackedCid]);
  const result = await reconcileOwnedContainers({ docker, owned, runId });
  assert.deepEqual(result, { remaining: [], reconciled: 2 });
  assert.equal(owned.size, 0);
  assert.equal(containers.size, 0);
  const psCalls = calls.filter((c) => c[0] === "ps");
  assert.equal(psCalls.length, 2);
  assert.ok(psCalls.every((c) => c.includes("--no-trunc")));
  assert.ok(calls.some((c) => c[0] === "container" && c[1] === "rm" && c[2] === trackedCid));
  assert.ok(calls.some((c) => c[0] === "container" && c[1] === "stop" && c[3] === lostCid));
  assert.ok(calls.some((c) => c[0] === "container" && c[1] === "rm" && c[2] === lostCid));
});

test("reconcileOwnedContainers refuses to stop or remove a foreign-labeled container", async () => {
  const calls = [];
  const docker = async (...args) => {
    calls.push(args);
    if (args[0] === "ps") return "foreign-cid-1\n";
    if (args[0] === "container" && args[1] === "inspect") {
      return JSON.stringify([
        {
          Id: "foreign-cid-1",
          Config: { Labels: { "bff-template.runtime-run": "other-run" } },
          State: { Running: true },
        },
      ]);
    }
    throw new Error(`must not mutate foreign container: ${args.join(" ")}`);
  };

  await assert.rejects(
    () => reconcileOwnedContainers({ docker, owned: new Set(), runId: "my-run" }),
    /refuse cleanup of a foreign container/,
  );
  assert.equal(
    calls.some((c) => c[1] === "stop" || c[1] === "rm"),
    false,
  );
});
