// Runtime readback boundary for spec009 T018. Unit fixtures are not Docker evidence.
import { isDeepStrictEqual } from "node:util";

const DEFAULT_DOCKER_HOST = "unix:///var/run/docker.sock";
const MIN_DOCKER_API_MAJOR = 1;
const MIN_DOCKER_API_MINOR = 41;
const SHA256_RE = /^sha256:[0-9a-f]{64}$/;
const GIT_SHA_RE = /^[0-9a-f]{40}$/;

export function resolveLocalDockerHost(raw) {
  if (raw === undefined || raw === "") return DEFAULT_DOCKER_HOST;
  if (typeof raw !== "string" || /\s/.test(raw) || !raw.startsWith("unix:///")) {
    throw new Error("docker host must be a local unix:// socket");
  }
  const socketPath = raw.slice("unix://".length);
  const segments = socketPath.split("/");
  if (socketPath.length <= 1 || segments.includes("..")) {
    throw new Error("docker host must be a local unix:// socket");
  }
  return raw;
}

export function checkDaemonVersion(version) {
  const server = version?.Server;
  const serverVersion = typeof server?.Version === "string" ? server.Version.trim() : "";
  const serverApiVersion = typeof server?.ApiVersion === "string" ? server.ApiVersion.trim() : "";
  const match = /^(\d+)\.(\d+)$/.exec(serverApiVersion);
  if (!serverVersion || !match) {
    throw new Error("docker daemon unavailable or version metadata missing");
  }
  const major = Number(match[1]);
  const minor = Number(match[2]);
  if (
    major < MIN_DOCKER_API_MAJOR ||
    (major === MIN_DOCKER_API_MAJOR && minor < MIN_DOCKER_API_MINOR)
  ) {
    throw new Error(`incompatible docker daemon API version: ${serverApiVersion}`);
  }
  return { serverVersion, serverApiVersion };
}

export function checkRuntimeReceipt(receipt, expected) {
  if (!receipt || typeof receipt !== "object") {
    throw new Error("runtime terminal receipt contract mismatch");
  }
  resolveLocalDockerHost(receipt.dockerHost);
  if (
    !GIT_SHA_RE.test(expected.revision ?? "") ||
    !GIT_SHA_RE.test(expected.sourceTree ?? "") ||
    !SHA256_RE.test(expected.manifestDigest ?? "") ||
    !SHA256_RE.test(expected.configDigest ?? "")
  ) {
    throw new Error("runtime terminal receipt contract mismatch");
  }
  checkDaemonVersion({
    Server: { Version: receipt.serverVersion, ApiVersion: receipt.serverApiVersion },
  });
  const actual = {
    schemaVersion: receipt.schemaVersion,
    revision: receipt.revision,
    sourceTree: receipt.sourceTree,
    sourceDirty: receipt.sourceDirty,
    manifestDigest: receipt.manifestDigest,
    configDigest: receipt.configDigest,
    teardownVerified: receipt.teardownVerified,
    ownedContainersRemaining: receipt.ownedContainersRemaining,
  };
  const wanted = {
    schemaVersion: 1,
    revision: expected.revision,
    sourceTree: expected.sourceTree,
    sourceDirty: false,
    manifestDigest: expected.manifestDigest,
    configDigest: expected.configDigest,
    teardownVerified: true,
    ownedContainersRemaining: 0,
  };
  if (!isDeepStrictEqual(actual, wanted)) {
    throw new Error("runtime terminal receipt contract mismatch");
  }
}

function parseContainerIds(output) {
  return String(output ?? "")
    .trim()
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

export async function reconcileOwnedContainers({ docker, owned, runId }) {
  if (typeof runId !== "string" || runId.length === 0) {
    throw new Error("run ownership ID is required for container reconciliation");
  }
  const filter = `label=bff-template.runtime-run=${runId}`;
  const discovered = parseContainerIds(
    await docker("ps", "-a", "--no-trunc", "--filter", filter, "--format", "{{.ID}}"),
  );
  const candidates = new Set([...owned, ...discovered]);
  for (const cid of candidates) {
    const [value] = JSON.parse(await docker("container", "inspect", cid));
    if (value?.Config?.Labels?.["bff-template.runtime-run"] !== runId) {
      throw new Error("refuse cleanup of a foreign container");
    }
    if (value.State?.Running) await docker("container", "stop", "--time=10", cid);
    await docker("container", "rm", cid);
    owned.delete(cid);
  }
  const remaining = parseContainerIds(
    await docker("ps", "-a", "--no-trunc", "--filter", filter, "--format", "{{.ID}}"),
  );
  return { remaining, reconciled: candidates.size };
}

function processConfig(value) {
  return { user: value?.User, entrypoint: value?.Entrypoint, command: value?.Cmd ?? [] };
}
const productionProcess = { user: "65532:65532", entrypoint: ["/bff"], command: [] };
export function checkLoadedImage(value, identity) {
  const actual = {
    id: value.Id,
    os: value.Os,
    architecture: value.Architecture,
    process: processConfig(value.Config),
    rootfs: value.RootFS,
  };
  const expected = {
    id: identity.configDigest,
    os: "linux",
    architecture: "amd64",
    process: productionProcess,
    rootfs: { Type: "layers", Layers: identity.diffIDs },
  };
  if (!isDeepStrictEqual(actual, expected)) throw new Error("loaded image identity mismatch");
}
export const emptyAssets = { "/web": "ro,noexec,nosuid,nodev,size=1m,mode=0555" };
function hostConfig(h) {
  return {
    network: h?.NetworkMode,
    readonly: h?.ReadonlyRootfs,
    capDrop: h?.CapDrop,
    security: h?.SecurityOpt,
    binds: h?.Binds ?? [],
    tmpfs: h?.Tmpfs ?? {},
  };
}
export function checkContainer(value, identity, owner, missingAssets) {
  const actual = {
    id: value.Id,
    name: value.Name,
    image: value.Image,
    owner: value.Config?.Labels?.["bff-template.runtime"],
    process: processConfig(value.Config),
    ...hostConfig(value.HostConfig),
    mounts: value.Mounts?.filter(
      (m) => !(missingAssets && m.Type === "tmpfs" && m.Destination === "/web"),
    ),
  };
  const expected = {
    id: owner.cid,
    name: "/" + owner.name,
    image: identity.configDigest,
    owner: owner.label,
    process: productionProcess,
    network: "host",
    readonly: true,
    capDrop: ["ALL"],
    security: ["no-new-privileges"],
    binds: [],
    tmpfs: missingAssets ? emptyAssets : {},
    mounts: [],
  };
  if (owner.runId !== undefined) {
    actual.runId = value.Config?.Labels?.["bff-template.runtime-run"];
    expected.runId = owner.runId;
  }
  if (!isDeepStrictEqual(actual, expected)) throw new Error("container runtime contract mismatch");
}
export function checkReady(event, port) {
  const addr = event.addr;
  if (
    event.event !== "ready" ||
    event.mode !== "production" ||
    event.identity !== "verified" ||
    ![`0.0.0.0:${port}`, `[::]:${port}`].includes(addr)
  )
    throw new Error("PORT listener mismatch: " + JSON.stringify(event));
}
