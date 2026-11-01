// The defect witness (specs/005-browser-client FR-007). The same specs run against a server that
// maps not_found to internal. Exactly the scenarios that observe not found must fail, each from an
// assertion and not from a crash. A run that fails everything would prove nothing.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import path from "node:path";

const report = path.join(process.env.TEST_TMPDIR, "defect-report.json");
const cli = createRequire(import.meta.url).resolve("@playwright/test/cli");
const run = spawnSync(
  process.execPath,
  [cli, "test", "--config", "web/e2e/playwright.config.mjs"],
  {
    env: { ...process.env, E2E_REPORT: report },
    stdio: ["ignore", "inherit", "inherit"],
    timeout: 120000,
  },
);
assert.equal(run.status, 1, "the specs must fail against the broken server");

const results = [];
const visit = (suite) => {
  for (const spec of suite.specs ?? []) {
    results.push(spec);
  }
  (suite.suites ?? []).forEach(visit);
};
JSON.parse(readFileSync(report, "utf8")).suites.forEach(visit);

const failed = results.filter((spec) => !spec.ok);
assert.deepEqual(failed.map((spec) => spec.title).sort(), [
  "another principal's note reads as not found",
  "not found",
]);
assert.equal(results.length, 18);
for (const spec of failed) {
  const message = spec.tests[0].results[0].error.message;
  assert.match(message, /Not found\./, "the failure must be the visible text, not a crash");
}
console.log("defect witness passed: exactly the not-found scenarios failed");
