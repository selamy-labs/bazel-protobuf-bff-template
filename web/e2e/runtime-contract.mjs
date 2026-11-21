// Runtime readback boundary for spec009 T018. Unit fixtures are not Docker evidence.
import { isDeepStrictEqual } from "node:util";

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
