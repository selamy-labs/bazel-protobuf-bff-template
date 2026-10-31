// Starts the real pieces for one test worker: the static page first, because its origin is what the
// server must allow, then the real server binary. Ports come from the kernel.
import { spawn } from "node:child_process";
import path from "node:path";

import { serve } from "../serve/serve.mjs";

const required = (name) => {
  const value = process.env[name];
  if (!value) {
    throw new Error(`${name} is not set; run this through bazel test`);
  }
  return value;
};

export function browserLaunchOptions() {
  if (process.platform !== "linux" || process.arch !== "x64") {
    throw new Error("the pinned browser exists only for Linux x86_64 (specs/005-browser-client research R1)");
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

function ready(child) {
  return new Promise((resolve, reject) => {
    let output = "";
    let errors = "";
    child.stderr.on("data", (chunk) => {
      errors += chunk;
    });
    child.stdout.on("data", (chunk) => {
      output += chunk;
      const newline = output.indexOf("\n");
      if (newline !== -1) {
        resolve(JSON.parse(output.slice(0, newline)));
      }
    });
    child.once("exit", (code) => reject(new Error(`the server exited with ${code}: ${errors}`)));
  });
}

export async function startStack() {
  const site = await serve(required("WEB_DIST"));
  const server = spawn(path.resolve(required("BFF_BIN")), ["--addr=127.0.0.1:0", `--allowed-origin=${site.origin}`], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  const { addr } = await ready(server);
  const bffUrl = `http://${addr}`;
  site.setBffUrl(bffUrl);
  return {
    origin: site.origin,
    bffUrl,
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
