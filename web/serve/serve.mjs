// Serves a built page under a base path, and its config.json, which tells the page where the server is. It is
// for local use and tests: a deployment puts the bundle on its own static host. The base path is required, as a
// Pages project site serves under one (spec 012 FR-019): "/" for the local demo, the site's path for the hosted
// example. Nothing outside the base path is served.
//
//   node serve.mjs --root <dir> --port <port> --base-path <path> --bff-url <url>
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

const types = {
  ".html": "text/html",
  ".js": "text/javascript",
  ".css": "text/css",
};

export async function serve(root, port, basePath) {
  if (!Number.isInteger(port) || port < 0 || port > 65535) {
    throw new Error(`serve: port must be a whole number from 0 (ephemeral) to 65535, not ${port}`);
  }
  if (typeof basePath !== "string" || !/^\/([a-z0-9-]+\/)*$/.test(basePath)) {
    throw new Error(
      `serve: base path must start and end with a slash, such as / or /site/, not ${basePath}`,
    );
  }
  const config = { bffUrl: "" };
  const base = path.resolve(root);
  const server = createServer(async (request, response) => {
    const url = new URL(request.url, "http://localhost");
    if (!url.pathname.startsWith(basePath)) {
      response.writeHead(404).end();
      return;
    }
    const under = url.pathname.slice(basePath.length);
    if (under === "config.json") {
      response.writeHead(200, { "Content-Type": "application/json" }).end(JSON.stringify(config));
      return;
    }
    const file = path.resolve(base, under === "" ? "index.html" : under);
    if (file !== base && !file.startsWith(base + path.sep)) {
      response.writeHead(403).end();
      return;
    }
    try {
      const body = await readFile(file);
      response
        .writeHead(200, {
          "Content-Type": types[path.extname(file)] ?? "application/octet-stream",
        })
        .end(body);
    } catch {
      response.writeHead(404).end();
    }
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, "127.0.0.1", resolve);
  });
  return {
    origin: `http://127.0.0.1:${server.address().port}`,
    basePath,
    setBffUrl: (url) => {
      config.bffUrl = url;
    },
    // The production page signs in with this issuer as this client (spec 012 FR-016); the dev page ignores it.
    setSignIn: (signIn) => {
      if (signIn === undefined) {
        delete config.signIn;
        return;
      }
      const { issuer, clientId } = signIn;
      config.signIn = { issuer, clientId };
    },
    close: () =>
      new Promise((resolve) => {
        server.close(resolve);
        server.closeAllConnections();
      }),
  };
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const option = (name) => process.argv[process.argv.indexOf(name) + 1];
  const site = await serve(option("--root"), Number(option("--port")), option("--base-path"));
  site.setBffUrl(option("--bff-url"));
  console.log(
    JSON.stringify({
      event: "ready",
      origin: site.origin,
      basePath: site.basePath,
    }),
  );
  for (const signal of ["SIGINT", "SIGTERM"]) {
    process.on(signal, () => site.close().then(() => process.exit(0)));
  }
}
