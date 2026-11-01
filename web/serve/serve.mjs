// Serves a built page and /config.json, which tells the page where the server is. It is for local
// use and tests: a deployment puts the bundle on its own static host.
//
//   node serve.mjs --root <dir> --port <port> --bff-url <url>
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import path from "node:path";
import { fileURLToPath } from "node:url";

const types = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" };

export async function serve(root, port = 0) {
  const config = { bffUrl: "" };
  const base = path.resolve(root);
  const server = createServer(async (request, response) => {
    const url = new URL(request.url, "http://localhost");
    if (url.pathname === "/config.json") {
      response.writeHead(200, { "Content-Type": "application/json" }).end(JSON.stringify(config));
      return;
    }
    const relative = url.pathname === "/" ? "index.html" : url.pathname.slice(1);
    const file = path.resolve(base, relative);
    if (file !== base && !file.startsWith(base + path.sep)) {
      response.writeHead(403).end();
      return;
    }
    try {
      const body = await readFile(file);
      response
        .writeHead(200, { "Content-Type": types[path.extname(file)] ?? "application/octet-stream" })
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
    setBffUrl: (url) => {
      config.bffUrl = url;
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
  const site = await serve(option("--root"), Number(option("--port")));
  site.setBffUrl(option("--bff-url"));
  console.log(JSON.stringify({ event: "ready", origin: site.origin }));
  for (const signal of ["SIGINT", "SIGTERM"]) {
    process.on(signal, () => site.close().then(() => process.exit(0)));
  }
}
