// Positive, negative, and derivative-adoption verification for BFF Playwright video evidence (#27).
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { cp, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  NOTES_REFERENCE_FLOW,
  NOTES_SYNTHETIC_SEED,
  resolveVerifiedBeforeBuild,
  runNotesReferenceFlow,
} from "./record-notes-video.mjs";
import {
  DIST_PROVENANCE_SCHEMA_VERSION,
  VIEWPORTS,
  createVideoPlaywrightConfig,
  hashDirectoryTree,
  parseWebmVideoTrack,
  recordWebFlowEvidence,
  validateVideoEvidence,
  verifyDistProvenance,
} from "./video-evidence.mjs";

const BEFORE_REVISION = "1111111111111111111111111111111111111111";
const AFTER_REVISION = "2222222222222222222222222222222222222222";
const WRONG_REVISION = "3333333333333333333333333333333333333333";

function sha256(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

async function copyBundle(t, srcDir) {
  const clone = await mkdtemp(path.join(process.env.TEST_TMPDIR ?? os.tmpdir(), "video-neg-"));
  t.after(() => rm(clone, { recursive: true, force: true }));
  await cp(srcDir, clone, { recursive: true });
  return clone;
}

async function mutateManifest(dir, mutator) {
  const manifestPath = path.join(dir, "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  await mutator(manifest, dir);
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, "utf8");
  return manifestPath;
}

async function startDerivativePage(label = "Derivative Flow") {
  const html = `<!doctype html>
<html lang="en">
  <body>
    <main>
      <h1>${label}</h1>
      <button id="toggle" type="button">Filter active</button>
      <p data-testid="state">all</p>
    </main>
    <script>
      const button = document.getElementById("toggle");
      const out = document.querySelector("[data-testid='state']");
      button.addEventListener("click", () => {
        out.textContent = out.textContent === "all" ? "active-only" : "all";
      });
    </script>
  </body>
</html>`;
  const server = createServer((_request, response) => {
    response.writeHead(200, { "Content-Type": "text/html" }).end(html);
  });
  await new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", resolve);
  });
  return {
    origin: `http://127.0.0.1:${server.address().port}`,
    close: () =>
      new Promise((resolve) => {
        server.close(resolve);
        server.closeAllConnections();
      }),
  };
}

test("records and validates representative smoke and provenance-bound comparative fixture and rejects defects", async (t) => {
  const webDist = process.env.WEB_DIST;
  assert.ok(webDist, "WEB_DIST must be set by Bazel");
  const afterDistSha = await hashDirectoryTree(webDist);

  // Synthetic provenance-bound two-build comparative fixture (distinct from CI's one-revision smoke
  // and from web_video_evidence.sh's detached git worktree baseline build).
  const beforeWork = await mkdtemp(
    path.join(process.env.TEST_TMPDIR ?? os.tmpdir(), "before-dist-"),
  );
  t.after(() => rm(beforeWork, { recursive: true, force: true }));
  const beforeDist = path.join(beforeWork, "dist");
  await cp(webDist, beforeDist, { recursive: true, dereference: true });
  await writeFile(path.join(beforeDist, "baseline-marker.txt"), "baseline-build-bytes\n", "utf8");
  const beforeDistSha = await hashDirectoryTree(beforeDist);
  assert.notEqual(beforeDistSha, afterDistSha);

  const beforeProvPath = path.join(beforeWork, "before-provenance.json");
  await writeFile(
    beforeProvPath,
    `${JSON.stringify({
      schemaVersion: DIST_PROVENANCE_SCHEMA_VERSION,
      revision: BEFORE_REVISION,
      tree: "4444444444444444444444444444444444444444",
      target: "//web/app:dist_dev",
      distSha256: beforeDistSha,
    })}\n`,
    "utf8",
  );

  // Provenance verification succeeds for matching baseline bytes and rejects mismatched build bytes
  // or mismatched tree SHAs even when paired with the exact expected baseline revision SHA.
  const verifiedBefore = await resolveVerifiedBeforeBuild({
    mode: "before-after",
    beforeRevision: BEFORE_REVISION,
    beforeTree: "4444444444444444444444444444444444444444",
    beforeWebDist: beforeDist,
    beforeProvenance: beforeProvPath,
  });
  assert.equal(verifiedBefore.beforeDistSha256, beforeDistSha);

  await assert.rejects(
    () =>
      resolveVerifiedBeforeBuild({
        mode: "before-after",
        beforeRevision: BEFORE_REVISION,
        beforeWebDist: beforeDist,
        beforeProvenance: beforeProvPath,
      }),
    /requires --before-revision, --before-tree, --before-web-dist, and --before-provenance/,
  );

  await assert.rejects(
    () =>
      resolveVerifiedBeforeBuild({
        mode: "before-after",
        beforeRevision: BEFORE_REVISION,
        beforeTree: "5555555555555555555555555555555555555555",
        beforeWebDist: beforeDist,
        beforeProvenance: beforeProvPath,
      }),
    /baseline provenance tree mismatch/,
  );

  await assert.rejects(
    () =>
      resolveVerifiedBeforeBuild({
        mode: "before-after",
        beforeRevision: BEFORE_REVISION,
        beforeTree: "4444444444444444444444444444444444444444",
        beforeWebDist: webDist,
        beforeProvenance: beforeProvPath,
      }),
    /baseline distSha256 mismatch/,
  );

  const tamperedDist = path.join(beforeWork, "tampered-dist");
  await cp(beforeDist, tamperedDist, { recursive: true });
  await writeFile(path.join(tamperedDist, "baseline-marker.txt"), "swapped-bytes\n", "utf8");
  await assert.rejects(
    () =>
      verifyDistProvenance({
        distDir: tamperedDist,
        provenancePath: beforeProvPath,
        expectedRevision: BEFORE_REVISION,
      }),
    /baseline distSha256 mismatch/,
  );

  // 1. Positive representative smoke (single-revision CI mode).
  const smokeDir = await mkdtemp(path.join(process.env.TEST_TMPDIR ?? os.tmpdir(), "video-smoke-"));
  t.after(() => rm(smokeDir, { recursive: true, force: true }));
  const smoke = await recordWebFlowEvidence({
    outputDir: smokeDir,
    revision: AFTER_REVISION,
    distSha256: afterDistSha,
    command: "tools/checks/web_video_evidence.sh",
    flow: NOTES_REFERENCE_FLOW,
    mode: "representative-smoke",
    viewports: [VIEWPORTS.desktop, VIEWPORTS.mobile],
    seedState: NOTES_SYNTHETIC_SEED,
    runFlow: ({ page, capture, seedState }) =>
      runNotesReferenceFlow({ page, capture, webDist, seedState }),
  });
  const smokeVerified = await validateVideoEvidence({
    manifestPath: smoke.manifestPath,
    expectedRevision: AFTER_REVISION,
    expectedDistSha256: afterDistSha,
    expectedFlow: NOTES_REFERENCE_FLOW,
    requireBeforeAfter: false,
    requireMobile: true,
  });
  assert.equal(smokeVerified.videoCount, 2);
  assert.equal(smokeVerified.screenshotCount, 12);

  // 2. Positive comparative before-after fixture binding both revision SHAs and distribution digests.
  const root = await mkdtemp(path.join(process.env.TEST_TMPDIR ?? os.tmpdir(), "video-evidence-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const { manifestPath } = await recordWebFlowEvidence({
    outputDir: root,
    revision: AFTER_REVISION,
    beforeRevision: BEFORE_REVISION,
    distSha256: afterDistSha,
    beforeDistSha256: beforeDistSha,
    command: "tools/checks/web_video_evidence.sh --mode before-after",
    flow: NOTES_REFERENCE_FLOW,
    mode: "before-after",
    viewports: [VIEWPORTS.desktop, VIEWPORTS.mobile],
    seedState: NOTES_SYNTHETIC_SEED,
    runFlow: ({ page, capture, phase, seedState }) =>
      runNotesReferenceFlow({
        page,
        capture,
        webDist: phase === "before" ? beforeDist : webDist,
        seedState,
      }),
  });

  const positive = await validateVideoEvidence({
    manifestPath,
    expectedRevision: AFTER_REVISION,
    expectedBeforeRevision: BEFORE_REVISION,
    expectedDistSha256: afterDistSha,
    expectedBeforeDistSha256: beforeDistSha,
    expectedFlow: NOTES_REFERENCE_FLOW,
    requireBeforeAfter: true,
    requireMobile: true,
  });
  assert.equal(positive.ok, true);
  assert.equal(positive.videoCount, 4);
  assert.equal(positive.screenshotCount, 24);

  // Verify actual WebM track properties directly.
  const desktopVideo = await readFile(path.join(root, "desktop/after/flow.webm"));
  const track = parseWebmVideoTrack(desktopVideo);
  assert.equal(track.width, 1280);
  assert.equal(track.height, 720);
  assert.match(track.codecId, /^V_/);

  // Negative 1: missing beforeRevision or beforeDistSha256 rejected in comparative mode.
  await assert.rejects(
    () =>
      recordWebFlowEvidence({
        outputDir: root,
        revision: AFTER_REVISION,
        beforeRevision: BEFORE_REVISION,
        distSha256: afterDistSha,
        command: "tools/checks/web_video_evidence.sh --mode before-after",
        flow: NOTES_REFERENCE_FLOW,
        mode: "before-after",
        viewports: [VIEWPORTS.desktop],
        seedState: NOTES_SYNTHETIC_SEED,
        runFlow: async () => {},
      }),
    /requires a verified 64-hex beforeDistSha256/,
  );

  // Negative 2: missing manifest rejected.
  await assert.rejects(
    () => validateVideoEvidence({ manifestPath: path.join(root, "missing.json") }),
    /ENOENT/,
  );

  // Negative 3: wrong candidate revision, wrong baseline revision, and mismatched baseline bytes
  // paired with the correct expected baseline SHA are all rejected.
  await assert.rejects(
    () =>
      validateVideoEvidence({
        manifestPath,
        expectedRevision: WRONG_REVISION,
        expectedBeforeRevision: BEFORE_REVISION,
        requireBeforeAfter: true,
      }),
    /revision mismatch/,
  );
  await assert.rejects(
    () =>
      validateVideoEvidence({
        manifestPath,
        expectedRevision: AFTER_REVISION,
        expectedBeforeRevision: WRONG_REVISION,
        requireBeforeAfter: true,
      }),
    /beforeRevision mismatch/,
  );
  await assert.rejects(
    () =>
      validateVideoEvidence({
        manifestPath,
        expectedRevision: AFTER_REVISION,
        expectedBeforeRevision: BEFORE_REVISION,
        expectedBeforeDistSha256: afterDistSha,
        requireBeforeAfter: true,
      }),
    /beforeDistSha256 mismatch/,
  );

  // Negative 4: wrong-size real WebM video with consistent manifest digests rejected.
  const wrongSizeDir = await mkdtemp(
    path.join(process.env.TEST_TMPDIR ?? os.tmpdir(), "video-640-"),
  );
  t.after(() => rm(wrongSizeDir, { recursive: true, force: true }));
  const derivative = await startDerivativePage("Small Video");
  t.after(() => derivative.close());
  await recordWebFlowEvidence({
    outputDir: wrongSizeDir,
    revision: AFTER_REVISION,
    command: "wrong-size",
    flow: "wrong-size-flow",
    mode: "representative-smoke",
    viewports: [{ name: "small", width: 640, height: 480 }],
    seedState: {},
    runFlow: async ({ page, capture }) => {
      await page.goto(derivative.origin);
      await capture("start");
    },
  });
  const wrongSizeBytes = await readFile(path.join(wrongSizeDir, "small/smoke/flow.webm"));
  const wrongSizeBundle = await copyBundle(t, root);
  const wrongSizeManifest = await mutateManifest(wrongSizeBundle, async (manifest, dir) => {
    const rel = manifest.viewports[0].captures.after.video.path;
    await writeFile(path.join(dir, rel), wrongSizeBytes);
    manifest.viewports[0].captures.after.video.bytes = wrongSizeBytes.length;
    manifest.viewports[0].captures.after.video.sha256 = sha256(wrongSizeBytes);
  });
  await assert.rejects(
    () =>
      validateVideoEvidence({
        manifestPath: wrongSizeManifest,
        expectedRevision: AFTER_REVISION,
        expectedBeforeRevision: BEFORE_REVISION,
      }),
    /video track dimensions 640x480 != 1280x720/,
  );

  // Negative 5: missing video file rejected.
  const missingVideoDir = await copyBundle(t, root);
  await rm(path.join(missingVideoDir, "desktop/after/flow.webm"));
  await assert.rejects(
    () =>
      validateVideoEvidence({
        manifestPath: path.join(missingVideoDir, "manifest.json"),
        expectedRevision: AFTER_REVISION,
        expectedBeforeRevision: BEFORE_REVISION,
        requireBeforeAfter: true,
      }),
    /ENOENT/,
  );

  // Negative 6: non-WebM video rejected even when manifest digest is updated.
  const corruptVideoDir = await copyBundle(t, root);
  const badVideoPath = await mutateManifest(corruptVideoDir, async (manifest, dir) => {
    const fakeBytes = Buffer.alloc(2048, 0x41);
    const rel = manifest.viewports[0].captures.after.video.path;
    await writeFile(path.join(dir, rel), fakeBytes);
    manifest.viewports[0].captures.after.video.bytes = fakeBytes.length;
    manifest.viewports[0].captures.after.video.sha256 = sha256(fakeBytes);
  });
  await assert.rejects(
    () =>
      validateVideoEvidence({
        manifestPath: badVideoPath,
        expectedRevision: AFTER_REVISION,
        expectedBeforeRevision: BEFORE_REVISION,
      }),
    /not a valid non-empty WebM video/,
  );

  // Negative 7: mismatched before/after starting state rejected.
  const mismatchStateDir = await copyBundle(t, root);
  const mismatchStatePath = await mutateManifest(mismatchStateDir, (manifest) => {
    manifest.viewports[0].captures.after.startingStateHash = "0".repeat(64);
  });
  await assert.rejects(
    () =>
      validateVideoEvidence({
        manifestPath: mismatchStatePath,
        expectedRevision: AFTER_REVISION,
        expectedBeforeRevision: BEFORE_REVISION,
        requireBeforeAfter: true,
      }),
    /startingStateHash mismatch/,
  );

  // Negative 8: missing mobile viewport rejected when requireMobile is true.
  const noMobileDir = await copyBundle(t, root);
  const noMobilePath = await mutateManifest(noMobileDir, (manifest) => {
    manifest.viewports = manifest.viewports.filter((v) => v.width > 480);
  });
  await assert.rejects(
    () =>
      validateVideoEvidence({
        manifestPath: noMobilePath,
        expectedRevision: AFTER_REVISION,
        expectedBeforeRevision: BEFORE_REVISION,
        requireMobile: true,
      }),
    /mobile viewport capture is required/,
  );

  // Negative 9: oversized total budget rejected.
  await assert.rejects(
    () =>
      validateVideoEvidence({
        manifestPath,
        expectedRevision: AFTER_REVISION,
        expectedBeforeRevision: BEFORE_REVISION,
        limits: { maxTotalBytes: 1024 },
      }),
    /total evidence size .* exceeds 1024/,
  );

  // Negative 10: secret marker and false hosted claim rejected.
  const secretDir = await copyBundle(t, root);
  const plantedMarker = ["-----BEGIN", " PRIVATE KEY-----"].join("");
  const secretPath = await mutateManifest(secretDir, (manifest) => {
    manifest.seedState.leaked = plantedMarker;
  });
  await assert.rejects(
    () => validateVideoEvidence({ manifestPath: secretPath, expectedRevision: AFTER_REVISION }),
    /forbidden secret/,
  );

  const falseHostedDir = await copyBundle(t, root);
  const falseHostedPath = await mutateManifest(falseHostedDir, (manifest) => {
    manifest.evidenceBoundary = "hosted-production";
  });
  await assert.rejects(
    () =>
      validateVideoEvidence({ manifestPath: falseHostedPath, expectedRevision: AFTER_REVISION }),
    /synthetic deterministic UI boundary/,
  );
});

test("derivative web app adopts the template Playwright config and recorder without copying a framework", async (t) => {
  const dir = await mkdtemp(path.join(process.env.TEST_TMPDIR ?? os.tmpdir(), "video-deriv-"));
  t.after(() => rm(dir, { recursive: true, force: true }));
  const derivative = await startDerivativePage();
  t.after(() => derivative.close());

  const config = createVideoPlaywrightConfig({
    outputDir: dir,
    viewport: VIEWPORTS.desktop,
  });
  assert.equal(config.use.video.mode, "on");
  assert.deepEqual(config.use.viewport, { width: 1280, height: 720 });

  const { manifestPath } = await recordWebFlowEvidence({
    outputDir: dir,
    revision: AFTER_REVISION,
    command: "derivative-app/record.sh",
    flow: "derivative-filter-smoke",
    mode: "representative-smoke",
    viewports: [VIEWPORTS.desktop],
    seedState: { items: ["alpha", "beta"] },
    runFlow: async ({ page, capture }) => {
      await page.goto(derivative.origin);
      await capture("initial");
      await page.getByRole("button", { name: "Filter active" }).click();
      await page.getByTestId("state").waitFor();
      assert.equal(await page.getByTestId("state").textContent(), "active-only");
      await capture("filtered");
    },
  });

  const verified = await validateVideoEvidence({
    manifestPath,
    expectedRevision: AFTER_REVISION,
    expectedFlow: "derivative-filter-smoke",
    requireBeforeAfter: false,
    requireMobile: false,
  });
  assert.equal(verified.videoCount, 1);
  assert.equal(verified.screenshotCount, 2);
});
