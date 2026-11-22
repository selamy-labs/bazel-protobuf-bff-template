// Source/browser witness for spec009 T016/T017. Uses the production binary and built UI on
// one origin with the existing external signed local issuer. This is not a container witness.
import { spawn } from "node:child_process";
import { cp, mkdtemp, rm } from "node:fs/promises";
import { createServer } from "node:net";
import path from "node:path";
import { createInterface } from "node:readline";
import { setTimeout as delay } from "node:timers/promises";

import { test as base, expect } from "@playwright/test";

import { startIssuer } from "./issuer.mjs";
import { sameOriginJourney } from "./same-origin-journey.mjs";

async function freePort() {
  const probe = createServer();
  await new Promise((resolve, reject) => {
    probe.once("error", reject);
    probe.listen(0, "127.0.0.1", resolve);
  });
  const port = probe.address().port;
  await new Promise((resolve, reject) =>
    probe.close((error) => (error ? reject(error) : resolve())),
  );
  return port;
}

async function ready(child, errors) {
  const lines = createInterface({ input: child.stdout });
  let timer;
  try {
    return await new Promise((resolve, reject) => {
      timer = setTimeout(
        () => reject(new Error(`single-origin startup timed out: ${errors.text}`)),
        10000,
      );
      lines.once("line", (line) => {
        try {
          resolve(JSON.parse(line));
        } catch (error) {
          reject(error);
        }
      });
      child.once("error", reject);
      child.once("exit", (code) =>
        reject(new Error(`single-origin server exited ${code}: ${errors.text}`)),
      );
    });
  } finally {
    clearTimeout(timer);
    lines.close();
  }
}

async function stop(child) {
  if (!child.pid || child.exitCode !== null || child.signalCode !== null) return;
  const exited = new Promise((resolve) => child.once("exit", () => resolve(true)));
  child.kill("SIGTERM");
  if (await Promise.race([exited, delay(5000, false, { ref: false })])) return;
  child.kill("SIGKILL");
  if (!(await Promise.race([exited, delay(2000, false, { ref: false })]))) {
    throw new Error("single-origin server did not terminate after SIGKILL");
  }
}

const test = base.extend({
  stack: [
    async ({}, use) => {
      const issuer = await startIssuer("single-origin-browser");
      let webRoot;
      let child;
      try {
        const port = await freePort();
        const origin = `http://127.0.0.1:${port}`;
        // Bazel's sandbox represents tree input files with links outside the input directory.
        // Copy the declared built bytes into owned regular files; retain production confinement.
        webRoot = await mkdtemp(path.join(process.env.TEST_TMPDIR, "same-origin-assets-"));
        await cp(path.resolve(process.env.WEB_DIST_PRODUCTION), webRoot, {
          recursive: true,
          dereference: true,
        });
        const errors = { text: "" };
        child = spawn(
          path.resolve(process.env.BFF_PRODUCTION),
          [
            `--allowed-origin=${origin}`,
            `--web-root=${webRoot}`,
            `--verifier-issuer=${issuer.origin}`,
            "--verifier-audience=single-origin-browser",
            `--verifier-jwks-url=${issuer.jwksUrl}`,
            "--signin-client-id=single-origin-browser",
            "--store=memory",
          ],
          {
            stdio: ["ignore", "pipe", "pipe"],
            env: {
              ...process.env,
              PORT: String(port),
              BFF_ADDR: "",
              // Existing source-journey telemetry fixture; not a release identity or export witness.
              BFF_OTEL_ENDPOINT: "http://127.0.0.1:1",
              BFF_RELEASE_REVISION: "0000000000000000000000000000000000000000",
              BFF_OTEL_ENVIRONMENT: "dev",
              BFF_OTEL_SAMPLE_RATIO: "0",
            },
          },
        );
        child.stderr.on("data", (chunk) => {
          errors.text = (errors.text + chunk).slice(-65536);
        });
        const event = await ready(child, errors);
        if (
          event.event !== "ready" ||
          ![`0.0.0.0:${port}`, `[::]:${port}`].includes(event.addr) ||
          event.mode !== "production" ||
          event.identity !== "verified"
        )
          throw new Error("unexpected single-origin listener");
        console.log("same-origin PORT listener", event.addr, event.mode, event.identity);
        await use({ origin, issuer });
      } finally {
        try {
          if (child) await stop(child);
        } finally {
          try {
            await issuer.close();
          } finally {
            if (webRoot) await rm(webRoot, { recursive: true, force: true });
          }
        }
      }
    },
    { scope: "worker" },
  ],
  notes: async ({ page, stack }, use) => {
    await page.goto(stack.origin + "/");
    await expect(page.getByTestId("sign-in")).toBeVisible();
    await use(page);
  },
});

sameOriginJourney(test);
