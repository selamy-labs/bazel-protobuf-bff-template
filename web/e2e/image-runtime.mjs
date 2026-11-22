// Actual rules_img archive -> local Docker -> immutable configuration -> browser fixture.
// Docker CLI contracts: https://docs.docker.com/reference/cli/docker/image/load/ and
// https://docs.docker.com/reference/cli/docker/container/run/ . No registry or daemon changes.
import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { createReadStream } from "node:fs";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:net";
import os from "node:os";
import path from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { promisify } from "node:util";

import { readImageIdentity } from "./image-identity.mjs";
import { checkContainer, checkLoadedImage, emptyAssets } from "./runtime-contract.mjs";

const execute = promisify(execFile);
export async function freePort() {
  const server = createServer();
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  const port = server.address().port;
  await new Promise((resolve, reject) =>
    server.close((error) => (error ? reject(error) : resolve())),
  );
  return port;
}
async function digestFile(file) {
  const hash = createHash("sha256");
  for await (const chunk of createReadStream(file)) hash.update(chunk);
  return "sha256:" + hash.digest("hex");
}
async function checkArchive(archive, identity, env) {
  const run = async (entry) =>
    (
      await execute("tar", ["-xOf", archive, "--", entry], {
        env,
        timeout: 30000,
        maxBuffer: 2 << 20,
      })
    ).stdout;
  const manifests = JSON.parse(await run("manifest.json"));
  assert.equal(manifests.length, 1, "one archive image required");
  const entry = manifests[0].Config;
  assert.match(entry, /^(?:blobs\/sha256\/)?[a-f0-9]{64}(?:\.json)?$/, "archive config path");
  const config = await run(entry);
  assert.equal(
    "sha256:" + createHash("sha256").update(config).digest("hex"),
    identity.configDigest,
    "archive/source configuration binding",
  );
  assert.deepEqual(JSON.parse(config).rootfs, { type: "layers", diff_ids: identity.diffIDs });
  assert.equal(manifests[0].Layers.length, identity.diffIDs.length, "archive layers");
}
function containerEnvironment({
  issuer,
  otlp,
  revision,
  origin,
  port,
  otherPort,
  wrongPort,
  brokenRPC,
  missingVerifier,
}) {
  const values = {
    PORT: String(port),
    BFF_ALLOWED_ORIGIN: origin,
    BFF_VERIFIER_ISSUER: issuer.origin,
    BFF_VERIFIER_AUDIENCE: issuer.clientId,
    BFF_VERIFIER_JWKS_URL: issuer.jwksUrl,
    BFF_SIGNIN_CLIENT_ID: issuer.clientId,
    BFF_OTEL_ENDPOINT: otlp,
    BFF_RELEASE_REVISION: revision,
    BFF_OTEL_ENVIRONMENT: "dev",
    BFF_OTEL_SAMPLE_RATIO: "0",
  };
  if (wrongPort) values.BFF_ADDR = `0.0.0.0:${otherPort}`;
  if (brokenRPC) {
    values.BFF_DEFAULT_DEADLINE = "1ns";
    values.BFF_MAX_DEADLINE = "1ns";
  }
  if (missingVerifier) {
    delete values.BFF_VERIFIER_ISSUER;
    delete values.BFF_VERIFIER_AUDIENCE;
    delete values.BFF_VERIFIER_JWKS_URL;
    delete values.BFF_SIGNIN_CLIENT_ID;
  }
  return values;
}
export async function openImageRuntime() {
  assert.equal(process.platform, "linux");
  assert.equal(process.arch, "x64");
  const revision = process.env.BFF_IMAGE_REVISION;
  assert.match(revision ?? "", /^[a-f0-9]{40}$/, "exact source revision required");
  const outputs = path.resolve(process.env.BFF_IMAGE_RUNTIME_OUTPUTS);
  await mkdir(outputs, { recursive: true });
  const identity = await readImageIdentity(path.resolve(process.env.BFF_IMAGE_LAYOUT));
  const archive = path.resolve(process.env.BFF_IMAGE_ARCHIVE);
  const privateConfig = await mkdtemp(path.join(os.tmpdir(), "bff-docker-config-"));
  // Closed subprocess environment: no ambient Docker contexts, registry credentials or cloud tokens.
  const env = { PATH: process.env.PATH, HOME: privateConfig, DOCKER_CONFIG: privateConfig };
  const docker = async (...args) => {
    const result = await execute("docker", ["--host=unix:///var/run/docker.sock", ...args], {
      env,
      timeout: 30000,
      maxBuffer: 4 << 20,
    });
    return args[1] === "logs" ? result.stdout + result.stderr : result.stdout;
  };
  const save = async (name, value) =>
    writeFile(path.join(outputs, name), JSON.stringify(value, null, 2) + "\n");
  try {
    await checkArchive(archive, identity, env);
    const version = JSON.parse(await docker("version", "--format={{json .}}"));
    const archiveDigest = await digestFile(archive);
    await writeFile(
      path.join(outputs, "load.log"),
      await docker("image", "load", "--input", archive),
    );
    const [image] = JSON.parse(await docker("image", "inspect", identity.configDigest));
    checkLoadedImage(image, identity);
    await save("image-identity.json", { revision, ...identity, archiveDigest, version, image });
  } catch (error) {
    await rm(privateConfig, { recursive: true, force: true });
    throw error;
  }
  async function start({
    issuer,
    otlp,
    missingAssets = false,
    wrongPort = false,
    brokenRPC = false,
    missingVerifier = false,
  }) {
    const port = await freePort(),
      otherPort = wrongPort ? await freePort() : port;
    assert.ok(!wrongPort || otherPort !== port, "distinct negative-control port");
    const origin = `http://127.0.0.1:${port}`;
    const name = "bff-image-runtime-" + randomUUID();
    const owner = { name, label: randomUUID(), cid: "" };
    const arguments_ = [
      "container",
      "create",
      "--name",
      name,
      "--label",
      "bff-template.runtime=" + owner.label,
      "--pull=never",
      "--network=host",
      "--read-only",
      "--cap-drop=ALL",
      "--security-opt=no-new-privileges",
      "--pids-limit=64",
    ];
    if (missingAssets) arguments_.push("--tmpfs", "/web:" + emptyAssets["/web"]);
    const values = containerEnvironment({
      issuer,
      otlp,
      revision,
      origin,
      port,
      otherPort,
      wrongPort,
      brokenRPC,
      missingVerifier,
    });
    for (const [key, value] of Object.entries(values)) arguments_.push("--env", key + "=" + value);
    arguments_.push(identity.configDigest);
    owner.cid = (await docker(...arguments_)).trim();
    assert.match(owner.cid, /^[a-f0-9]{64}$/, "created container ID");
    const inspect = async () => JSON.parse(await docker("container", "inspect", owner.cid))[0];
    const checked = async () => {
      const value = await inspect();
      checkContainer(value, identity, owner, missingAssets);
      return value;
    };
    async function close() {
      let value = await checked();
      if (value.State.Running) await docker("container", "stop", "--time=10", owner.cid);
      value = await checked();
      await save(name + "-terminal.json", value);
      await writeFile(
        path.join(outputs, name + ".log"),
        await docker("container", "logs", owner.cid),
      );
      assert.equal(value.State.Running, false, "owned container terminal");
      // Remove only the exact, ownership-checked terminal container; never force/prune/cache cleanup.
      await docker("container", "rm", owner.cid);
      assert.equal(value.State.ExitCode, missingVerifier ? 1 : 0, "expected production exit");
    }
    async function awaitStartup() {
      const end = Date.now() + 15000;
      while (Date.now() < end) {
        const state = await checked();
        const logs = await docker("container", "logs", owner.cid);
        const first = logs.split("\n").find((line) => line.startsWith('{"event":"ready"'));
        if (first) {
          const event = JSON.parse(first);
          await save(name + "-started.json", {
            revision,
            identity,
            owner,
            port,
            otherPort,
            missingAssets,
            wrongPort,
            brokenRPC,
            event,
            inspect: state,
          });
          return { origin, port, otherPort, event, close };
        }
        if (missingVerifier && !state.State.Running) {
          assert.equal(state.State.ExitCode, 1, "missing verifier startup exit");
          assert.equal(
            logs,
            "bff: no verifier is configured: name the issuer with --verifier-issuer, --verifier-audience and --verifier-jwks-url (or BFF_VERIFIER_ISSUER, BFF_VERIFIER_AUDIENCE, BFF_VERIFIER_JWKS_URL); this repository ships no identity provider\n",
          );
          await save(name + "-refused.json", {
            revision,
            identity,
            owner,
            port,
            inspect: state,
            logs,
          });
          return { origin, port, refused: true, close };
        }
        assert.ok(state.State.Running, "image exited before production ready event");
        await delay(100);
      }
      throw new Error("actual image startup timed out");
    }
    try {
      await checked();
      await docker("container", "start", owner.cid);
      return await awaitStartup();
    } catch (error) {
      await close();
      throw error;
    }
  }
  return { identity, start, close: () => rm(privateConfig, { recursive: true, force: true }) };
}
