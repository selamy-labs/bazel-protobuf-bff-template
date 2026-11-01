// The whole stack through the one command (spec 012 FR-020 to FR-023, SC-006): the real launcher starts the
// real server and the real page, a real browser signs in, creates a note, opens it, reloads and opens it again;
// the launcher is stopped and restarted and the note is still there; a reset clears it. The startup time is
// measured from the launcher's start to both ports answering, and written beside the report.
import { spawn } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { test as base, expect } from "@playwright/test";

const required = (name) => {
  const value = process.env[name];
  if (!value) {
    throw new Error(`${name} is not set; run this through bazel test`);
  }
  return value;
};

const randomPort = (base) => base + Math.floor(Math.random() * 10000);

async function reachable(url) {
  try {
    const response = await fetch(url, { signal: AbortSignal.timeout(1000) });
    return response.status > 0;
  } catch {
    return false;
  }
}

// launch runs the launcher as `bazel run //tools/dev` would, with the profile's values overridden by name.
async function launch(ports, dataDir, extra = []) {
  const started = process.hrtime.bigint();
  const child = spawn(
    path.resolve(required("DEV_SH")),
    [...required("DEV_ARGS").split(" "), ...extra],
    {
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
    },
  );
  let output = "";
  child.stdout.on("data", (chunk) => {
    output += chunk;
  });
  child.stderr.on("data", (chunk) => {
    output += chunk;
  });
  const origin = `http://127.0.0.1:${ports.web}`;
  const bff = `http://127.0.0.1:${ports.bff}`;
  for (let i = 0; i < 600; i++) {
    if (child.exitCode !== null) {
      throw new Error(`the launcher exited with ${child.exitCode}:\n${output}`);
    }
    if ((await reachable(`${origin}/config.json`)) && (await reachable(`${bff}/`))) {
      const startupMs = Number(process.hrtime.bigint() - started) / 1e6;
      return {
        origin,
        startupMs,
        output: () => output,
        stop: async () => {
          const exited = new Promise((resolve) => child.once("exit", resolve));
          process.kill(-child.pid, "SIGTERM");
          await exited;
          for (let j = 0; j < 200; j++) {
            if (!(await reachable(`${origin}/`)) && !(await reachable(`${bff}/`))) {
              return;
            }
            await new Promise((resolve) => setTimeout(resolve, 20));
          }
          throw new Error("a child survived the launcher's stop");
        },
      };
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error(`the stack did not come up:\n${output}`);
}

const test = base.extend({
  demo: [
    async ({}, use) => {
      const ports = { bff: randomPort(20000), web: randomPort(30000) };
      const dataDir = path.join(
        mkdtempSync(path.join(process.env.TEST_TMPDIR ?? tmpdir(), "demo-")),
        "notes",
      );
      await use({ ports, dataDir });
    },
    { scope: "worker" },
  ],
});

const signIn = (page, subject) => page.getByLabel("Test identity").fill(subject);

async function createNote(page, title, body) {
  await page.getByLabel("Title").fill(title);
  await page.getByLabel("Body").fill(body);
  await page.getByRole("button", { name: "Create note" }).click();
  await expect(page.getByRole("heading", { name: title })).toBeVisible();
  return page.getByTestId("note-name").textContent();
}

async function openNote(page, name) {
  await page.getByLabel("Name").fill(name);
  await page.getByRole("button", { name: "Open note" }).click();
}

test("one command brings the whole stack up; a note survives a restart and not a reset", async ({
  page,
  demo,
}) => {
  test.setTimeout(120000);
  const first = await launch(demo.ports, demo.dataDir);
  expect(first.output()).toContain(`notes persist in ${demo.dataDir}`);
  expect(first.output()).toContain(`store="local" # from BFF_STORE`);
  const browserLog = [];
  page.on("console", (message) => browserLog.push(`console ${message.type()}: ${message.text()}`));
  page.on("pageerror", (error) => browserLog.push(`pageerror: ${error.message}`));
  page.on("requestfailed", (request) =>
    browserLog.push(`requestfailed: ${request.url()} ${request.failure()?.errorText}`),
  );
  page.on("response", (response) => {
    if (response.status() >= 400)
      browserLog.push(`response ${response.status()}: ${response.url()}`);
  });
  await page.goto(first.origin);
  await expect(
    page.getByTestId("test-identity"),
    `launcher:\n${first.output()}\nbrowser:\n${browserLog.join("\n")}`,
  ).toBeVisible();
  await signIn(page, "ana");
  const name = await createNote(page, "Demo", "survives a restart");
  expect(name).toMatch(/^notes\/[a-z0-9-]+$/);
  await page.reload();
  await signIn(page, "ana");
  await openNote(page, name);
  await expect(page.getByTestId("note-body")).toHaveText("survives a restart");
  await first.stop();

  const second = await launch(demo.ports, demo.dataDir);
  await page.goto(second.origin);
  await signIn(page, "ana");
  await openNote(page, name);
  await expect(page.getByTestId("note-body")).toHaveText("survives a restart");
  await signIn(page, "bob");
  await openNote(page, name);
  await expect(page.getByTestId("failure")).toHaveText("Not found.");
  await second.stop();

  const third = await launch(demo.ports, demo.dataDir, ["--reset"]);
  expect(third.output()).toContain(`reset: removed ${demo.dataDir}`);
  await page.goto(third.origin);
  await signIn(page, "ana");
  await openNote(page, name);
  await expect(page.getByTestId("failure")).toHaveText("Not found.");
  await third.stop();

  const record = {
    command: "tools/bootstrap/bazel.sh run //tools/dev",
    startup_ms: [first.startupMs, second.startupMs, third.startupMs].map((ms) => Math.round(ms)),
    platform: `${process.platform}/${process.arch}`,
    node: process.version,
  };
  const outputs = process.env.TEST_UNDECLARED_OUTPUTS_DIR ?? tmpdir();
  writeFileSync(path.join(outputs, "local-stack.json"), JSON.stringify(record, null, 2));
  console.log(`local-stack: ${JSON.stringify(record)}`);
});
