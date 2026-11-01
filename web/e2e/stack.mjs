// Starts the real pieces for one test worker: the static page first, because its origin is what the
// server must allow, then the real server binary. Ports come from the kernel.
import { spawn } from "node:child_process";
import path from "node:path";

import { serve } from "../serve/serve.mjs";
import { startIssuer } from "./issuer.mjs";

const required = (name) => {
  const value = process.env[name];
  if (!value) {
    throw new Error(`${name} is not set; run this through bazel test`);
  }
  return value;
};

export function browserLaunchOptions() {
  if (process.platform !== "linux" || process.arch !== "x64") {
    throw new Error(
      "the pinned browser exists only for Linux x86_64 (specs/005-browser-client research R1)",
    );
  }
  const browser = path.resolve(required("BROWSER_DIR"));
  return {
    executablePath: path.join(browser, "chrome-headless-shell"),
    env: {
      ...process.env,
      LD_LIBRARY_PATH: path.join(browser, "lib"),
      // Every fontconfig path resolves inside the pinned tree, so host fonts and /etc/fonts are out of reach.
      FONTCONFIG_SYSROOT: path.join(browser, "fontconfig-root"),
      HOME: process.env.TEST_TMPDIR ?? process.env.HOME,
    },
  };
}

function ready(child, errors) {
  return new Promise((resolve, reject) => {
    let output = "";
    child.stdout.on("data", (chunk) => {
      output += chunk;
      const newline = output.indexOf("\n");
      if (newline !== -1) {
        resolve(JSON.parse(output.slice(0, newline)));
      }
    });
    child.once("exit", (code) =>
      reject(new Error(`the server exited with ${code}: ${errors.text}`)),
    );
  });
}

// basePath is where the page is served, "/" or a site's path (spec 012 FR-019); the backend is always on its
// own origin. The backend's stderr is kept, so a test can read what it logged.
export async function startStack(basePath) {
  const site = await serve(required("WEB_DIST"), 0, basePath);
  const errors = { text: "" };
  const server = spawn(
    path.resolve(required("BFF_BIN")),
    ["--addr=127.0.0.1:0", `--allowed-origin=${site.origin}`, "--store=memory"],
    {
      stdio: ["ignore", "pipe", "pipe"],
    },
  );
  server.stderr.on("data", (chunk) => {
    errors.text += chunk;
  });
  const { addr } = await ready(server, errors);
  const bffUrl = `http://${addr}`;
  site.setBffUrl(bffUrl);
  return {
    origin: site.origin,
    basePath,
    bffUrl,
    log: () => errors.text,
    stopServer: async () => {
      server.removeAllListeners("exit");
      const exited = new Promise((resolve) => server.once("exit", resolve));
      server.kill("SIGTERM");
      await exited;
    },
    stop: async () => {
      server.removeAllListeners("exit");
      server.kill("SIGTERM");
      await site.close();
    },
  };
}

// The hosted shape with the sign-in (spec 012 FR-016, T014): the production bundle under the base path, a local
// issuer, and the production server with its verifier pointed at that issuer's key set. The client id the page
// signs in as is the audience the server accepts: one value, as the hosted inputs declare it once.
export async function startProductionStack(basePath) {
  const clientId = "bff-hosted-example";
  const issuer = await startIssuer(clientId);
  const site = await serve(required("WEB_DIST_PRODUCTION"), 0, basePath);
  const errors = { text: "" };
  const server = spawn(
    path.resolve(required("BFF_PRODUCTION")),
    [
      "--addr=127.0.0.1:0",
      `--allowed-origin=${site.origin}`,
      "--store=memory",
      `--verifier-issuer=${issuer.origin}`,
      `--verifier-audience=${clientId}`,
      `--verifier-jwks-url=${issuer.jwksUrl}`,
    ],
    { stdio: ["ignore", "pipe", "pipe"] },
  );
  server.stderr.on("data", (chunk) => {
    errors.text += chunk;
  });
  const { addr } = await ready(server, errors);
  const bffUrl = `http://${addr}`;
  site.setBffUrl(bffUrl);
  site.setSignIn({ issuer: issuer.origin, clientId });
  return {
    origin: site.origin,
    basePath,
    bffUrl,
    issuer,
    log: () => errors.text,
    stop: async () => {
      server.removeAllListeners("exit");
      server.kill("SIGTERM");
      await site.close();
      await issuer.close();
    },
  };
}
