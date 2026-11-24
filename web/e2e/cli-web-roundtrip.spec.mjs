// Cross-client CLI <-> web Notes round-trip proof (#29, specs/013-reference-clients):
// starts the real //tools/dev local stack (bff-dev + web/serve with local store), proves CLI->web
// and web->CLI create/read-by-ID, persistence across full stack restart, typed cross-subject
// refusal in both clients, and failed-start cleanup safety.
import { spawn, spawnSync } from "node:child_process";
import { mkdtempSync } from "node:fs";
import http from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";

import { expect, test as base } from "@playwright/test";

const required = (name) => {
  const value = process.env[name];
  if (!value) {
    throw new Error(`${name} is not set; run this through bazel test`);
  }
  return value;
};

const randomPort = (basePort) => basePort + Math.floor(Math.random() * 10000);
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function reachable(url) {
  try {
    const response = await fetch(url, { signal: AbortSignal.timeout(1000) });
    return response.status > 0;
  } catch {
    return false;
  }
}

function killGroup(pid, signal) {
  if (!pid) {
    return;
  }
  try {
    process.kill(-pid, signal);
  } catch (err) {
    if (err && err.code === "ESRCH") {
      return;
    }
    throw err;
  }
}

function groupAlive(pid) {
  if (!pid) {
    return false;
  }
  try {
    process.kill(-pid, 0);
    return true;
  } catch (err) {
    if (err && err.code === "ESRCH") {
      return false;
    }
    throw err;
  }
}

function waitForExit(child, timeoutMs) {
  if (child.exitCode !== null || child.signalCode !== null) {
    return Promise.resolve(true);
  }
  return new Promise((resolve) => {
    const finish = () => {
      clearTimeout(timer);
      child.off("exit", finish);
      child.off("error", finish);
      resolve(true);
    };
    const timer = setTimeout(() => {
      child.off("exit", finish);
      child.off("error", finish);
      resolve(false);
    }, timeoutMs);
    child.once("exit", finish);
    child.once("error", finish);
  });
}

// Purely observational post-retirement check: never signals a retired process group ID.
async function waitPortsClosed(origin, bff, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const originOpen = await reachable(`${origin}/`);
    const bffOpen = await reachable(`${bff}/`);
    if (!originOpen && !bffOpen) {
      return;
    }
    await sleep(25);
  }
  const occupied = [];
  if (await reachable(`${origin}/`)) {
    occupied.push(origin);
  }
  if (await reachable(`${bff}/`)) {
    occupied.push(bff);
  }
  throw new Error(
    `endpoint port still occupied after owned process group retired: ${occupied.join(", ")}`,
  );
}

const DEFAULT_STOP_OPTIONS = {
  termTimeoutMs: 5000,
  killTimeoutMs: 3000,
  portTimeoutMs: 4000,
  checkGroupAlive: groupAlive,
  signalGroup: killGroup,
  getSpawnError: () => null,
};

function destroyStreams(child) {
  child.stdout?.destroy();
  child.stderr?.destroy();
}

async function waitGroupRetired(child, pid, timeoutMs, checkGroupAlive) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const exited = child.exitCode !== null || child.signalCode !== null;
    if (exited && !checkGroupAlive(pid)) {
      return true;
    }
    await sleep(25);
  }
  return (child.exitCode !== null || child.signalCode !== null) && !checkGroupAlive(pid);
}

async function retireProcessGroup(child, cfg) {
  cfg.signalGroup(child.pid, "SIGTERM");
  const termExited = await waitForExit(child, cfg.termTimeoutMs);
  if (
    termExited &&
    (await waitGroupRetired(
      child,
      child.pid,
      Math.min(cfg.termTimeoutMs, 1000),
      cfg.checkGroupAlive,
    ))
  ) {
    return;
  }
  cfg.signalGroup(child.pid, "SIGKILL");
  const killExited = await waitForExit(child, cfg.killTimeoutMs);
  if (!killExited) {
    throw new Error(`launcher pid ${child.pid} did not exit within bounded stop timeout`);
  }
  if (!(await waitGroupRetired(child, child.pid, cfg.killTimeoutMs, cfg.checkGroupAlive))) {
    throw new Error(`owned process group ${child.pid} survived stop`);
  }
}

async function stopStack(child, origin, bff, options = {}) {
  const cfg = { ...DEFAULT_STOP_OPTIONS, ...options };
  if (cfg.getSpawnError() || !child.pid) {
    destroyStreams(child);
    return;
  }
  try {
    await retireProcessGroup(child, cfg);
  } finally {
    destroyStreams(child);
  }
  await waitPortsClosed(origin, bff, cfg.portTimeoutMs);
}

async function waitForStackReady(child, origin, bff, readOutput, getSpawnError, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const spawnErr = getSpawnError();
    if (spawnErr) {
      throw new Error(`launcher failed to spawn: ${spawnErr.message}`, { cause: spawnErr });
    }
    if (child.exitCode !== null || child.signalCode !== null) {
      throw new Error(
        `launcher exited with ${child.exitCode ?? child.signalCode}:\n${readOutput()}`,
      );
    }
    if ((await reachable(`${origin}/config.json`)) && (await reachable(`${bff}/`))) {
      return;
    }
    await sleep(50);
  }
  throw new Error(`local stack did not start:\n${readOutput()}`);
}

async function launchStack(ports, dataDir, options = {}) {
  const command = options.command ?? path.resolve(required("DEV_SH"));
  const args = options.args ?? required("DEV_ARGS").split(" ");
  const readyTimeoutMs = options.readyTimeoutMs ?? 30000;
  const child = spawn(command, args, {
    detached: true,
    env: {
      ...process.env,
      BFF_PORT: String(ports.bff),
      WEB_PORT: String(ports.web),
      BFF_STORE: "local",
      BFF_DATA_DIR: dataDir,
      BUILD_WORKSPACE_DIRECTORY: process.cwd(),
    },
    stdio: ["ignore", "pipe", "pipe"],
  });
  let spawnError = null;
  child.once("error", (err) => {
    spawnError = err;
  });
  let output = "";
  child.stdout?.on("data", (chunk) => {
    output += chunk;
  });
  child.stderr?.on("data", (chunk) => {
    output += chunk;
  });
  const origin = `http://127.0.0.1:${ports.web}`;
  const bff = `http://127.0.0.1:${ports.bff}`;
  let stopping;
  const stop = () => {
    if (!stopping) {
      stopping = stopStack(child, origin, bff, {
        ...options.stopOptions,
        getSpawnError: () => spawnError,
      });
    }
    return stopping;
  };
  const handle = { origin, bff, pid: child.pid, stop };
  options.onAcquire?.(handle);
  try {
    await waitForStackReady(
      child,
      origin,
      bff,
      () => output,
      () => spawnError,
      readyTimeoutMs,
    );
    return handle;
  } catch (launchErr) {
    try {
      await handle.stop();
    } catch (stopErr) {
      throw new AggregateError(
        [launchErr, stopErr],
        `${launchErr.message}; cleanup failed: ${stopErr.message}`,
      );
    }
    throw launchErr;
  }
}

async function stopAllHandles(handles) {
  const failures = [];
  for (const handle of [...handles].reverse()) {
    try {
      await handle.stop();
    } catch (err) {
      failures.push(err);
    }
  }
  if (failures.length === 1) {
    throw failures[0];
  }
  if (failures.length > 1) {
    throw new AggregateError(
      failures,
      `${failures.length} owned stack handles failed during cleanup`,
    );
  }
}

function runCli(args) {
  const result = spawnSync(path.resolve(required("NOTES_CLI_BIN")), args, {
    encoding: "utf8",
    timeout: 15000,
  });
  if (result.error) {
    throw result.error;
  }
  return result;
}

const test = base.extend({
  stackEnv: [
    async ({}, use) => {
      const ports = { bff: randomPort(21000), web: randomPort(31000) };
      const dataDir = path.join(
        mkdtempSync(path.join(process.env.TEST_TMPDIR ?? tmpdir(), "cli-web-")),
        "notes",
      );
      const owned = [];
      const launch = (launchPorts = ports, launchDataDir = dataDir, options = {}) =>
        launchStack(launchPorts, launchDataDir, {
          ...options,
          onAcquire: (handle) => {
            owned.push(handle);
            options.onAcquire?.(handle);
          },
        });
      try {
        await use({ ports, dataDir, launch });
      } finally {
        await stopAllHandles(owned);
      }
    },
    { scope: "test" },
  ],
});

const signIn = (page, subject) => page.getByLabel("Test identity").fill(subject);

async function openNoteInWeb(page, name) {
  await page.getByLabel("Name").fill(name);
  await page.getByRole("button", { name: "Open note" }).click();
}

async function createNoteInWeb(page, title, body) {
  await page.getByLabel("Title").fill(title);
  await page.getByLabel("Body").fill(body);
  await page.getByRole("button", { name: "Create note" }).click();
  await expect(page.getByRole("heading", { name: title })).toBeVisible();
  return (await page.getByTestId("note-name").textContent()).trim();
}

test("CLI and web share Notes create/read-by-ID, restart persistence, and cross-subject refusal", async ({
  page,
  stackEnv,
}) => {
  let stack = await stackEnv.launch();

  // 1. CLI creates a note as ana; web opens it by ID.
  const cliCreated = runCli([
    "create",
    `--endpoint=${stack.bff}`,
    "--subject=ana",
    "--title=From Python CLI",
    "--body=Read by browser UI.",
    "--output=json",
  ]);
  expect(cliCreated.status, cliCreated.stderr).toBe(0);
  const cliNote = JSON.parse(cliCreated.stdout);
  expect(cliNote.name).toMatch(/^notes\/[a-z0-9-]+$/);

  await page.goto(stack.origin);
  await signIn(page, "ana");
  await openNoteInWeb(page, cliNote.name);
  await expect(page.getByRole("heading", { name: "From Python CLI" })).toBeVisible();
  await expect(page.getByText("Read by browser UI.")).toBeVisible();

  // 2. Web creates a second note as ana; CLI reads it by ID.
  const webNoteName = await createNoteInWeb(page, "From Web UI", "Read by Python CLI.");
  const cliReadWeb = runCli([
    "get",
    `--endpoint=${stack.bff}`,
    "--subject=ana",
    `--name=${webNoteName}`,
    "--output=json",
  ]);
  expect(cliReadWeb.status, cliReadWeb.stderr).toBe(0);
  const webNote = JSON.parse(cliReadWeb.stdout);
  expect(webNote.name).toBe(webNoteName);
  expect(webNote.title).toBe("From Web UI");
  expect(webNote.body).toBe("Read by Python CLI.");

  // 3. Restart the entire local stack; both clients still read both notes.
  await stack.stop();
  stack = await stackEnv.launch();

  const cliAfterRestart = runCli([
    "get",
    `--endpoint=${stack.bff}`,
    "--subject=ana",
    `--name=${webNoteName}`,
    "--output=json",
  ]);
  expect(cliAfterRestart.status, cliAfterRestart.stderr).toBe(0);
  expect(JSON.parse(cliAfterRestart.stdout).title).toBe("From Web UI");

  await page.goto(stack.origin);
  await signIn(page, "ana");
  await openNoteInWeb(page, cliNote.name);
  await expect(page.getByRole("heading", { name: "From Python CLI" })).toBeVisible();

  // 4. Typed cross-subject refusal: bob cannot read ana's notes in CLI or web.
  const cliDenied = runCli([
    "get",
    `--endpoint=${stack.bff}`,
    "--subject=bob",
    `--name=${cliNote.name}`,
    "--output=json",
  ]);
  expect(cliDenied.status).toBe(1);
  expect(JSON.parse(cliDenied.stderr).error.code).toBe("not_found");

  await signIn(page, "bob");
  await openNoteInWeb(page, cliNote.name);
  await expect(page.getByRole("alert")).toContainText(/not found/i);
});

test("failed stack start cleans up its process group and stop is safe after exit", async ({
  stackEnv,
}) => {
  // 1. Readiness timeout cleans up a live process group.
  let hangingHandle;
  await expect(
    stackEnv.launch(stackEnv.ports, stackEnv.dataDir, {
      command: process.execPath,
      args: ["-e", "setInterval(() => {}, 1000)"],
      readyTimeoutMs: 250,
      onAcquire: (handle) => {
        hangingHandle = handle;
      },
    }),
  ).rejects.toThrow(/local stack did not start/);
  expect(hangingHandle?.pid).toBeTruthy();
  expect(groupAlive(hangingHandle.pid)).toBe(false);
  await expect(hangingHandle.stop()).resolves.toBeUndefined();

  // 2. Launcher exits while a SIGTERM-ignoring non-listening grandchild stays in the same group;
  // stop must escalate to SIGKILL and confirm process-group retirement.
  const spawnResistantGrandchild = [
    "const { spawn } = require('node:child_process');",
    "const child = spawn(process.execPath, ['-e', \"process.on('SIGTERM', () => {}); setInterval(() => {}, 1000);\"], { stdio: 'ignore' });",
    "child.unref();",
    "setTimeout(() => process.exit(1), 50);",
  ].join(" ");
  let exitedHandle;
  await expect(
    stackEnv.launch(stackEnv.ports, stackEnv.dataDir, {
      command: process.execPath,
      args: ["-e", spawnResistantGrandchild],
      readyTimeoutMs: 2000,
      stopOptions: { termTimeoutMs: 150, killTimeoutMs: 2000, portTimeoutMs: 1000 },
      onAcquire: (handle) => {
        exitedHandle = handle;
      },
    }),
  ).rejects.toThrow(/launcher exited with 1/);
  expect(exitedHandle?.pid).toBeTruthy();
  expect(groupAlive(exitedHandle.pid)).toBe(false);
  await expect(exitedHandle.stop()).resolves.toBeUndefined();

  // 3. Closed HTTP listeners do not satisfy stop while the owned process group is still reported alive.
  const probeChild = spawn(process.execPath, ["-e", "process.exit(0)"], {
    detached: true,
    stdio: ["ignore", "pipe", "pipe"],
  });
  await waitForExit(probeChild, 2000);
  await expect(
    stopStack(
      probeChild,
      `http://127.0.0.1:${stackEnv.ports.web}`,
      `http://127.0.0.1:${stackEnv.ports.bff}`,
      {
        termTimeoutMs: 100,
        killTimeoutMs: 100,
        portTimeoutMs: 200,
        checkGroupAlive: () => true,
      },
    ),
  ).rejects.toThrow(/owned process group \d+ survived stop/);
  expect(groupAlive(probeChild.pid)).toBe(false);
});

test("post-retirement port poll is observational and teardown handles spawn or stop failures", async ({
  stackEnv,
}) => {
  // 1. After the owned group retires, an occupied port fails observationally with zero signals sent.
  const unrelatedServer = http.createServer((req, res) => {
    res.writeHead(200);
    res.end(req.method ?? "ok");
  });
  await new Promise((resolve) => unrelatedServer.listen(0, "127.0.0.1", resolve));
  const occupiedPort = unrelatedServer.address().port;
  const occupiedOrigin = `http://127.0.0.1:${occupiedPort}`;
  try {
    const retiredChild = spawn(process.execPath, ["-e", "process.exit(0)"], {
      detached: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    await waitForExit(retiredChild, 2000);
    let retiredObserved = false;
    const signalsAfterRetirement = [];
    const allSignals = [];
    await expect(
      stopStack(retiredChild, occupiedOrigin, `http://127.0.0.1:${stackEnv.ports.bff}`, {
        termTimeoutMs: 100,
        killTimeoutMs: 100,
        portTimeoutMs: 150,
        checkGroupAlive: (pid) => {
          const alive = groupAlive(pid);
          if (!alive) {
            retiredObserved = true;
          }
          return alive;
        },
        signalGroup: (pid, signal) => {
          allSignals.push(signal);
          if (retiredObserved) {
            signalsAfterRetirement.push(signal);
          }
          killGroup(pid, signal);
        },
      }),
    ).rejects.toThrow(/endpoint port still occupied after owned process group retired/);
    expect(retiredObserved).toBe(true);
    expect(allSignals).toEqual(["SIGTERM"]);
    expect(signalsAfterRetirement).toEqual([]);
  } finally {
    await new Promise((resolve) => unrelatedServer.close(resolve));
  }

  // 2. Unspawnable launcher routes its error through the fixture failure path without unhandled events.
  let missingHandle;
  await expect(
    stackEnv.launch(stackEnv.ports, stackEnv.dataDir, {
      command: path.join(stackEnv.dataDir, "nonexistent-launcher-binary"),
      args: [],
      readyTimeoutMs: 2000,
      onAcquire: (handle) => {
        missingHandle = handle;
      },
    }),
  ).rejects.toThrow(/launcher failed to spawn/);
  expect(missingHandle?.pid).toBeUndefined();
  await expect(missingHandle.stop()).resolves.toBeUndefined();

  // 3. A rejected stop does not prevent teardown from cleaning up remaining acquired handles,
  // and the synthetic child probe keeps an independent bounded cleanup owner even if a helper aborts early.
  const spawnProbeWithCustody = () => {
    const child = spawn(process.execPath, ["-e", "setInterval(() => {}, 1000)"], {
      detached: true,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stopPromise;
    const stopChild = () => {
      if (!stopPromise) {
        stopPromise = stopStack(
          child,
          `http://127.0.0.1:${stackEnv.ports.web}`,
          `http://127.0.0.1:${stackEnv.ports.bff}`,
          { termTimeoutMs: 500, killTimeoutMs: 500, portTimeoutMs: 200 },
        );
      }
      return stopPromise;
    };
    return { child, stopChild };
  };

  const failingSecondHandle = {
    stop: async () => {
      throw new Error("simulated second handle stop failure");
    },
  };

  // 3a. Production stopAllHandles retires the surviving child before fallback custody runs.
  const primaryProbe = spawnProbeWithCustody();
  try {
    let firstCleaned = false;
    const firstHandle = {
      stop: async () => {
        await primaryProbe.stopChild();
        firstCleaned = true;
      },
    };
    await expect(stopAllHandles([firstHandle, failingSecondHandle])).rejects.toThrow(
      /simulated second handle stop failure/,
    );
    expect(firstCleaned).toBe(true);
    expect(groupAlive(primaryProbe.child.pid)).toBe(false);
  } finally {
    await primaryProbe.stopChild();
  }

  // 3b. Independent custody retires the child even when a regression helper aborts on the first rejection.
  const earlyAbortProbe = spawnProbeWithCustody();
  const abortingStopAll = async (handles) => {
    for (const handle of [...handles].reverse()) {
      await handle.stop();
    }
  };
  let abortProbeFirstCleaned = false;
  try {
    const firstHandle = {
      stop: async () => {
        await earlyAbortProbe.stopChild();
        abortProbeFirstCleaned = true;
      },
    };
    await expect(abortingStopAll([firstHandle, failingSecondHandle])).rejects.toThrow(
      /simulated second handle stop failure/,
    );
    expect(abortProbeFirstCleaned).toBe(false);
    expect(groupAlive(earlyAbortProbe.child.pid)).toBe(true);
  } finally {
    await earlyAbortProbe.stopChild();
  }
  expect(groupAlive(earlyAbortProbe.child.pid)).toBe(false);
});
