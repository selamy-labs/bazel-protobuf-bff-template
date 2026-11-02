// Local production-binary -> OTLP collector -> debug exporter witness. No cloud credentials.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, readFile, writeFile, rm } from "node:fs/promises";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { setTimeout as delay } from "node:timers/promises";
import { startIssuer } from "../../web/e2e/issuer.mjs";

const [collectorPath, bffPath, revision] = process.argv.slice(2);
assert.ok(
  collectorPath && bffPath && /^[0-9a-f]{40}$/.test(revision ?? ""),
  "usage: node smoke.mjs <otelcol-0.164.0> <bff> <source-sha>",
);
const directory = await mkdtemp(join(tmpdir(), "bff-otel-smoke-"));
const children = [];
let issuer;
async function port() {
  const listener = createServer();
  listener.listen(0, "127.0.0.1");
  await once(listener, "listening");
  const value = listener.address().port;
  await new Promise((resolve) => listener.close(resolve));
  return value;
}
function launch(binary, args, env = process.env) {
  const process = spawn(resolve(binary), args, {
    env,
    stdio: ["ignore", "pipe", "pipe"],
  });
  const result = { process, stdout: "", stderr: "", exit: null };
  result.exit = new Promise((resolve) => {
    process.once("exit", (code) => resolve(code));
    process.once("error", () => resolve(-1));
  });
  process.stdout.on("data", (chunk) => {
    result.stdout += chunk;
  });
  process.stderr.on("data", (chunk) => {
    result.stderr += chunk;
  });
  children.push(result);
  return result;
}
async function until(check, description) {
  const end = Date.now() + 15000;
  while (Date.now() < end) {
    if (await check()) return;
    await delay(50);
  }
  throw new Error(`timed out: ${description}`);
}
async function stop(child) {
  if (child.process.exitCode !== null) return;
  child.process.kill("SIGTERM");
  const finished = await Promise.race([child.exit.then(() => true), delay(7000).then(() => false)]);
  if (!finished) {
    child.process.kill("SIGKILL");
    await child.exit;
  }
}
try {
  const otlp = await port(),
    metrics = await port();
  const config = (await readFile(new URL("collector.yaml", import.meta.url), "utf8"))
    .replace("127.0.0.1:4318", `127.0.0.1:${otlp}`)
    .replace("port: 8888", `port: ${metrics}`);
  await writeFile(join(directory, "collector.yaml"), config);
  const collector = launch(collectorPath, [`--config=${join(directory, "collector.yaml")}`]);
  const metricsURL = `http://127.0.0.1:${metrics}/metrics`;
  await until(
    () =>
      fetch(metricsURL)
        .then((r) => r.ok)
        .catch(() => false),
    "collector readiness",
  );
  issuer = await startIssuer("bff-local-smoke");
  const server = launch(
    bffPath,
    [
      "--addr=127.0.0.1:0",
      "--allowed-origin=http://127.0.0.1:9876",
      "--store=memory",
      `--verifier-issuer=${issuer.origin}`,
      "--verifier-audience=bff-local-smoke",
      `--verifier-jwks-url=${issuer.jwksUrl}`,
    ],
    {
      ...process.env,
      BFF_OTEL_ENDPOINT: `http://127.0.0.1:${otlp}`,
      BFF_RELEASE_REVISION: revision,
      BFF_OTEL_ENVIRONMENT: "dev",
      BFF_OTEL_SAMPLE_RATIO: "1",
      BFF_OTEL_METRIC_INTERVAL: "100ms",
      BFF_OTEL_BATCH_DELAY: "10ms",
    },
  );
  await until(() => server.stdout.includes("\n"), "BFF readiness");
  const { addr } = JSON.parse(server.stdout.split("\n")[0]);
  const bearer = issuer.mint({ sub: "PRIVATE_SMOKE_SUBJECT" });
  async function call(method, body) {
    const response = await fetch(`http://${addr}/template.notes.v1.NoteService/${method}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${bearer}`,
      },
      body: JSON.stringify(body),
    });
    assert.equal(response.status, 200);
    return response.json();
  }
  const created = await call("CreateNote", {
    note: { title: "smoke", body: "PRIVATE_SMOKE_BODY" },
  });
  await call("GetNote", { name: created.note.name });
  await stop(server);
  await until(
    async () =>
      /otelcol_exporter_sent_spans[^\n]* [1-9]/.test(await (await fetch(metricsURL)).text()),
    "exported traces",
  );
  await until(
    () => collector.stdout.includes("bff.store") && collector.stdout.includes("Trace ID"),
    "store span and trace id in local viewer",
  );
  const counters = await (await fetch(metricsURL)).text();
  assert.match(counters, /otelcol_exporter_sent_metric_points[^\n]* [1-9]/);
  for (const secret of [bearer, "PRIVATE_SMOKE_SUBJECT", "PRIVATE_SMOKE_BODY"]) {
    assert.ok(!collector.stdout.includes(secret));
    assert.ok(!server.stderr.includes(secret));
  }
  console.log(
    JSON.stringify({
      event: "live_trace_smoke_pass",
      revision,
      spans: "CreateNote/GetNote plus store",
      metrics: "exported",
      redaction: "passed",
    }),
  );
  console.log(
    counters
      .split("\n")
      .filter((line) => /^otelcol_(exporter_sent|receiver_accepted)/.test(line))
      .join("\n"),
  );
} finally {
  for (const child of children.reverse()) await stop(child);
  if (issuer) await issuer.close();
  await rm(directory, { recursive: true, force: true });
}
