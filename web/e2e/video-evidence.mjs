// Reusable template-owned Playwright video evidence recorder and fail-closed validator (#27).
// Strictly test tooling: production builds never import or execute this module.
import { createHash } from "node:crypto";
import {
  access,
  lstat,
  mkdir,
  readFile,
  readdir,
  rename,
  rm,
  stat,
  writeFile,
} from "node:fs/promises";
import path from "node:path";

import { browserLaunchOptions } from "./stack.mjs";

export const EVIDENCE_SCHEMA_VERSION = "bff-web-video-evidence/v1";
export const DIST_PROVENANCE_SCHEMA_VERSION = "bff-web-dist-provenance/v1";
export const EVIDENCE_BOUNDARY = "deterministic-ui-fake";
export const FFMPEG_REVISION = "1013";

export const VIEWPORTS = Object.freeze({
  desktop: Object.freeze({ name: "desktop", width: 1280, height: 720 }),
  mobile: Object.freeze({ name: "mobile", width: 390, height: 844 }),
});

export const DEFAULT_LIMITS = Object.freeze({
  maxVideoBytes: 5 * 1024 * 1024,
  maxScreenshotBytes: 2 * 1024 * 1024,
  maxTotalBytes: 20 * 1024 * 1024,
  minVideoBytes: 1024,
});

const REVISION_RE = /^[0-9a-f]{40}$/;
const SHA256_RE = /^[0-9a-f]{64}$/;
const SAFE_SEGMENT_RE = /^[a-z0-9][a-z0-9._-]*$/;
const WEBM_MAGIC = Buffer.from([0x1a, 0x45, 0xdf, 0xa3]);
const PNG_MAGIC = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
const EBML_SEGMENT_ID = 0x18538067;
const EBML_TRACKS_ID = 0x1654ae6b;
const EBML_TRACK_ENTRY_ID = 0xae;
const EBML_TRACK_TYPE_ID = 0x83;
const EBML_CODEC_ID = 0x86;
const EBML_VIDEO_ID = 0xe0;
const EBML_PIXEL_WIDTH_ID = 0xb0;
const EBML_PIXEL_HEIGHT_ID = 0xba;
const FORBIDDEN_SECRET_PATTERNS = [
  /-----BEGIN [A-Z ]*PRIVATE KEY-----/,
  /Bearer\s+[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/,
  /ghp_[A-Za-z0-9]{20,}/,
  /github_pat_[A-Za-z0-9_]{20,}/,
  /AIza[0-9A-Za-z_-]{30,}/,
];

function assertSafeSegment(value, label) {
  if (typeof value !== "string" || !SAFE_SEGMENT_RE.test(value)) {
    throw new Error(`video-evidence: invalid ${label}: ${String(value)}`);
  }
}

export function playwrightBrowsersPath() {
  const browserDir = process.env.BROWSER_DIR;
  if (!browserDir) {
    throw new Error("video-evidence: BROWSER_DIR is not set; run through Bazel");
  }
  return path.join(path.resolve(browserDir), "playwright-browsers");
}

export async function assertPinnedFfmpegAvailable() {
  const browsersDir = playwrightBrowsersPath();
  const ffmpegPath = path.join(browsersDir, `ffmpeg-${FFMPEG_REVISION}`, "ffmpeg-linux");
  await access(ffmpegPath);
  return { browsersDir, ffmpegPath };
}

export function createVideoPlaywrightConfig(overrides = {}) {
  const outputs = overrides.outputDir ?? process.env.TEST_UNDECLARED_OUTPUTS_DIR ?? "test-results";
  const viewport = overrides.viewport ?? VIEWPORTS.desktop;
  const size = { width: viewport.width, height: viewport.height };
  return {
    forbidOnly: true,
    fullyParallel: false,
    outputDir: `${outputs}/playwright-video`,
    reporter: [
      ["list"],
      ["json", { outputFile: overrides.reportPath ?? `${outputs}/video-report.json` }],
    ],
    retries: 0,
    testDir: overrides.testDir ?? ".",
    testMatch: overrides.testMatch ?? "*.video.spec.mjs",
    timeout: overrides.timeout ?? 30000,
    use: {
      browserName: "chromium",
      deviceScaleFactor: 1,
      launchOptions: browserLaunchOptions(),
      screenshot: "on",
      video: { mode: "on", size },
      viewport: size,
    },
    workers: 1,
  };
}

function sha256Hex(buffer) {
  return createHash("sha256").update(buffer).digest("hex");
}

function canonicalJsonHash(value) {
  return sha256Hex(Buffer.from(JSON.stringify(value), "utf8"));
}

async function collectFiles(rootDir, relDir, out) {
  const current = relDir === "" ? rootDir : path.join(rootDir, relDir);
  const entries = await readdir(current);
  for (const name of entries) {
    const rel = relDir === "" ? name : `${relDir}/${name}`;
    const full = path.join(rootDir, rel);
    const info = await stat(full);
    if (info.isDirectory()) {
      await collectFiles(rootDir, rel, out);
    } else if (info.isFile()) {
      out.push(rel);
    }
  }
}

export async function hashDirectoryTree(distDir) {
  const resolved = path.resolve(distDir);
  const info = await stat(resolved);
  if (!info.isDirectory()) {
    throw new Error(`video-evidence: distribution path is not a directory: ${resolved}`);
  }
  const files = [];
  await collectFiles(resolved, "", files);
  if (files.length === 0) {
    throw new Error(`video-evidence: distribution directory is empty: ${resolved}`);
  }
  files.sort((a, b) => (a < b ? -1 : a > b ? 1 : 0));
  const lines = [];
  for (const rel of files) {
    const data = await readFile(path.join(resolved, rel));
    lines.push(`${sha256Hex(data)}  ${rel}\n`);
  }
  return sha256Hex(Buffer.from(lines.join(""), "utf8"));
}

function assertProvenanceMetadata(provenance, expectedRevision, expectedTree, expectedTarget) {
  if (provenance.schemaVersion !== DIST_PROVENANCE_SCHEMA_VERSION) {
    throw new Error(
      `video-evidence: unsupported dist provenance schemaVersion ${String(provenance.schemaVersion)}`,
    );
  }
  if (provenance.revision !== expectedRevision) {
    throw new Error(
      `video-evidence: baseline provenance revision mismatch (got ${String(provenance.revision)}, expected ${expectedRevision})`,
    );
  }
  if (!REVISION_RE.test(provenance.tree ?? "")) {
    throw new Error("video-evidence: baseline provenance missing valid 40-hex tree SHA");
  }
  if (expectedTree && provenance.tree !== expectedTree) {
    throw new Error(
      `video-evidence: baseline provenance tree mismatch (got ${String(provenance.tree)}, expected ${expectedTree})`,
    );
  }
  if (provenance.target !== expectedTarget) {
    throw new Error(
      `video-evidence: baseline provenance target mismatch (got ${String(provenance.target)}, expected ${expectedTarget})`,
    );
  }
}

export async function verifyDistProvenance({
  distDir,
  provenancePath,
  expectedRevision,
  expectedTree = null,
  expectedTarget = "//web/app:dist_dev",
}) {
  if (!REVISION_RE.test(expectedRevision ?? "")) {
    throw new Error(
      `video-evidence: invalid expected baseline revision: ${String(expectedRevision)}`,
    );
  }
  const raw = await readFile(path.resolve(provenancePath), "utf8");
  const provenance = JSON.parse(raw);
  assertProvenanceMetadata(provenance, expectedRevision, expectedTree, expectedTarget);
  if (!SHA256_RE.test(provenance.distSha256 ?? "")) {
    throw new Error("video-evidence: baseline provenance missing valid distSha256");
  }
  const actualSha = await hashDirectoryTree(distDir);
  if (actualSha !== provenance.distSha256) {
    throw new Error(
      `video-evidence: baseline distSha256 mismatch (served ${actualSha} != provenance ${provenance.distSha256})`,
    );
  }
  return { provenance, distSha256: actualSha };
}

function parsePngDimensions(buffer) {
  if (buffer.length < 24 || !buffer.subarray(0, 8).equals(PNG_MAGIC)) {
    throw new Error("video-evidence: artifact is not a valid PNG image");
  }
  const chunkType = buffer.subarray(12, 16).toString("ascii");
  if (chunkType !== "IHDR") {
    throw new Error("video-evidence: PNG missing IHDR header");
  }
  return { width: buffer.readUInt32BE(16), height: buffer.readUInt32BE(20) };
}

function readEbmlVint(buffer, offset, keepMarker) {
  if (offset >= buffer.length || buffer[offset] === 0) {
    return null;
  }
  const first = buffer[offset];
  let width = 1;
  let mask = 0x80;
  while ((first & mask) === 0 && width < 8) {
    mask >>= 1;
    width += 1;
  }
  if (offset + width > buffer.length) {
    return null;
  }
  let value = keepMarker ? first : first & (mask - 1);
  let allOnes = (first & (mask - 1)) === mask - 1;
  for (let i = 1; i < width; i += 1) {
    const byte = buffer[offset + i];
    value = value * 256 + byte;
    allOnes = allOnes && byte === 0xff;
  }
  return { value, next: offset + width, unknown: !keepMarker && allOnes };
}

function readEbmlElement(buffer, pos, limit) {
  const id = readEbmlVint(buffer, pos, true);
  if (!id || id.next > limit) {
    return null;
  }
  const size = readEbmlVint(buffer, id.next, false);
  if (!size) {
    return null;
  }
  const dataStart = size.next;
  const dataEnd = size.unknown ? limit : dataStart + size.value;
  if (dataEnd > limit || dataEnd < dataStart) {
    return null;
  }
  return { id: id.value, dataStart, dataEnd };
}

function readEbmlUint(buffer, start, end) {
  let value = 0;
  for (let i = start; i < end; i += 1) {
    value = value * 256 + buffer[i];
  }
  return value;
}

function parseEbmlVideoDimensions(buffer, start, end) {
  let width = 0;
  let height = 0;
  let pos = start;
  while (pos < end) {
    const el = readEbmlElement(buffer, pos, end);
    if (!el) {
      break;
    }
    if (el.id === EBML_PIXEL_WIDTH_ID) {
      width = readEbmlUint(buffer, el.dataStart, el.dataEnd);
    } else if (el.id === EBML_PIXEL_HEIGHT_ID) {
      height = readEbmlUint(buffer, el.dataStart, el.dataEnd);
    }
    pos = el.dataEnd;
  }
  return { width, height };
}

function parseEbmlTrackEntry(buffer, start, end) {
  let trackType = 0;
  let codecId = "";
  let dims = { width: 0, height: 0 };
  let pos = start;
  while (pos < end) {
    const el = readEbmlElement(buffer, pos, end);
    if (!el) {
      break;
    }
    if (el.id === EBML_TRACK_TYPE_ID) {
      trackType = readEbmlUint(buffer, el.dataStart, el.dataEnd);
    } else if (el.id === EBML_CODEC_ID) {
      codecId = buffer.subarray(el.dataStart, el.dataEnd).toString("ascii");
    } else if (el.id === EBML_VIDEO_ID) {
      dims = parseEbmlVideoDimensions(buffer, el.dataStart, el.dataEnd);
    }
    pos = el.dataEnd;
  }
  return { trackType, codecId, ...dims };
}

function findEbmlChild(buffer, start, end, targetId) {
  let pos = start;
  while (pos < end) {
    const el = readEbmlElement(buffer, pos, end);
    if (!el) {
      return null;
    }
    if (el.id === targetId) {
      return el;
    }
    pos = el.dataEnd;
  }
  return null;
}

export function parseWebmVideoTrack(buffer, minBytes = DEFAULT_LIMITS.minVideoBytes) {
  if (buffer.length < minBytes || !buffer.subarray(0, 4).equals(WEBM_MAGIC)) {
    throw new Error("video-evidence: artifact is not a valid non-empty WebM video");
  }
  const segment = findEbmlChild(buffer, 0, buffer.length, EBML_SEGMENT_ID);
  if (!segment) {
    throw new Error("video-evidence: WebM missing Segment container");
  }
  const tracks = findEbmlChild(buffer, segment.dataStart, segment.dataEnd, EBML_TRACKS_ID);
  if (!tracks) {
    throw new Error("video-evidence: WebM missing Tracks metadata");
  }
  let pos = tracks.dataStart;
  while (pos < tracks.dataEnd) {
    const entry = readEbmlElement(buffer, pos, tracks.dataEnd);
    if (!entry) {
      break;
    }
    if (entry.id === EBML_TRACK_ENTRY_ID) {
      const parsed = parseEbmlTrackEntry(buffer, entry.dataStart, entry.dataEnd);
      if (parsed.trackType === 1 && parsed.width > 0 && parsed.height > 0) {
        return { width: parsed.width, height: parsed.height, codecId: parsed.codecId };
      }
    }
    pos = entry.dataEnd;
  }
  throw new Error("video-evidence: WebM contains no valid video track");
}

async function resolveEvidenceFile(rootDir, relPath) {
  if (typeof relPath !== "string" || relPath === "" || path.isAbsolute(relPath)) {
    throw new Error(`video-evidence: unsafe artifact path: ${String(relPath)}`);
  }
  const parts = relPath.split("/");
  if (parts.some((part) => part === "" || part === "." || part === "..")) {
    throw new Error(`video-evidence: traversal in artifact path: ${relPath}`);
  }
  const resolvedRoot = path.resolve(rootDir);
  const fullPath = path.resolve(resolvedRoot, relPath);
  if (!fullPath.startsWith(`${resolvedRoot}${path.sep}`)) {
    throw new Error(`video-evidence: artifact escapes evidence root: ${relPath}`);
  }
  const info = await lstat(fullPath);
  if (info.isSymbolicLink() || !info.isFile()) {
    throw new Error(`video-evidence: artifact must be a regular non-symlink file: ${relPath}`);
  }
  return { fullPath, bytes: await readFile(fullPath) };
}

async function capturePhase({
  browser,
  outputDir,
  viewport,
  phase,
  phaseRevision,
  phaseDistSha256,
  seedState,
  runFlow,
}) {
  const phaseRelDir = `${viewport.name}/${phase}`;
  const phaseDir = path.join(outputDir, phaseRelDir);
  const rawVideoDir = path.join(phaseDir, ".raw-video");
  await mkdir(rawVideoDir, { recursive: true });
  const size = { width: viewport.width, height: viewport.height };
  const context = await browser.newContext({
    viewport: size,
    deviceScaleFactor: 1,
    recordVideo: { dir: rawVideoDir, size },
  });
  const screenshots = [];
  let videoPath = null;
  try {
    const page = await context.newPage();
    const video = page.video();
    let stepIndex = 0;
    const capture = async (stepName) => {
      assertSafeSegment(stepName, "screenshot step");
      stepIndex += 1;
      const fileName = `${String(stepIndex).padStart(2, "0")}-${stepName}.png`;
      const relPath = `${phaseRelDir}/${fileName}`;
      const fullPath = path.join(outputDir, relPath);
      await page.screenshot({ path: fullPath, fullPage: false });
      const data = await readFile(fullPath);
      const dims = parsePngDimensions(data);
      screenshots.push({
        step: stepName,
        path: relPath,
        mediaType: "image/png",
        width: dims.width,
        height: dims.height,
        bytes: data.length,
        sha256: sha256Hex(data),
      });
    };
    await runFlow({ page, context, viewport, phase, phaseRevision, seedState, capture });
    await page.close();
    videoPath = await video.path();
  } finally {
    await context.close();
  }
  const videoRelPath = `${phaseRelDir}/flow.webm`;
  const finalVideoPath = path.join(outputDir, videoRelPath);
  await rename(videoPath, finalVideoPath);
  await rm(rawVideoDir, { recursive: true, force: true });
  const videoData = await readFile(finalVideoPath);
  const track = parseWebmVideoTrack(videoData, DEFAULT_LIMITS.minVideoBytes);
  return {
    revision: phaseRevision,
    distSha256: phaseDistSha256,
    startingStateHash: canonicalJsonHash(seedState),
    video: {
      path: videoRelPath,
      mediaType: "video/webm",
      codecId: track.codecId,
      width: track.width,
      height: track.height,
      bytes: videoData.length,
      sha256: sha256Hex(videoData),
    },
    screenshots,
  };
}

function resolvePhases(mode) {
  if (mode === "before-after") {
    return ["before", "after"];
  }
  if (mode === "representative-smoke") {
    return ["smoke"];
  }
  throw new Error(`video-evidence: unsupported mode ${String(mode)}`);
}

function validateBeforeAfterRevisions(beforeRevision, distSha256, beforeDistSha256) {
  if (!REVISION_RE.test(beforeRevision ?? "")) {
    throw new Error(
      `video-evidence: before-after mode requires a 40-hex beforeRevision, got ${String(beforeRevision)}`,
    );
  }
  if (!SHA256_RE.test(beforeDistSha256 ?? "")) {
    throw new Error(
      "video-evidence: before-after mode requires a verified 64-hex beforeDistSha256",
    );
  }
  if (!SHA256_RE.test(distSha256 ?? "")) {
    throw new Error("video-evidence: before-after mode requires a verified 64-hex distSha256");
  }
}

function validateRevisionBindings(mode, revision, beforeRevision, distSha256, beforeDistSha256) {
  if (!REVISION_RE.test(revision ?? "")) {
    throw new Error(
      `video-evidence: revision must be a 40-hex commit SHA, got ${String(revision)}`,
    );
  }
  if (distSha256 !== null && distSha256 !== undefined && !SHA256_RE.test(distSha256)) {
    throw new Error(`video-evidence: invalid distSha256: ${String(distSha256)}`);
  }
  if (mode === "before-after") {
    validateBeforeAfterRevisions(beforeRevision, distSha256, beforeDistSha256);
    return;
  }
  if (beforeRevision || beforeDistSha256) {
    throw new Error(
      "video-evidence: beforeRevision and beforeDistSha256 are only valid in before-after mode",
    );
  }
}

function normalizeRecordOptions(options) {
  return {
    outputDir: options.outputDir,
    revision: options.revision,
    beforeRevision: options.beforeRevision ?? null,
    distSha256: options.distSha256 ?? null,
    beforeDistSha256: options.beforeDistSha256 ?? null,
    command: options.command,
    flow: options.flow,
    mode: options.mode ?? "representative-smoke",
    viewports: options.viewports ?? [VIEWPORTS.desktop, VIEWPORTS.mobile],
    seedState: options.seedState ?? {},
    runFlow: options.runFlow,
  };
}

async function captureAllViewports(browser, opts, phases) {
  const viewportEntries = [];
  for (const viewport of opts.viewports) {
    assertSafeSegment(viewport.name, "viewport name");
    const captures = {};
    for (const phase of phases) {
      const isBefore = phase === "before";
      captures[phase] = await capturePhase({
        browser,
        outputDir: opts.outputDir,
        viewport,
        phase,
        phaseRevision: isBefore ? opts.beforeRevision : opts.revision,
        phaseDistSha256: isBefore ? opts.beforeDistSha256 : opts.distSha256,
        seedState: opts.seedState,
        runFlow: opts.runFlow,
      });
    }
    viewportEntries.push({
      name: viewport.name,
      width: viewport.width,
      height: viewport.height,
      captures,
    });
  }
  return viewportEntries;
}

export async function recordWebFlowEvidence(rawOptions) {
  const opts = normalizeRecordOptions(rawOptions);
  const phases = resolvePhases(opts.mode);
  validateRevisionBindings(
    opts.mode,
    opts.revision,
    opts.beforeRevision,
    opts.distSha256,
    opts.beforeDistSha256,
  );
  assertSafeSegment(opts.flow, "flow");
  const { browsersDir } = await assertPinnedFfmpegAvailable();
  if (!process.env.PLAYWRIGHT_BROWSERS_PATH) {
    process.env.PLAYWRIGHT_BROWSERS_PATH = browsersDir;
  }
  const { chromium } = await import("@playwright/test");
  await mkdir(opts.outputDir, { recursive: true });
  const launch = browserLaunchOptions();
  const browser = await chromium.launch({
    ...launch,
    env: { ...launch.env, PLAYWRIGHT_BROWSERS_PATH: browsersDir },
  });
  let viewportEntries;
  try {
    viewportEntries = await captureAllViewports(browser, opts, phases);
  } finally {
    await browser.close();
  }
  const isComparative = opts.mode === "before-after";
  const manifest = {
    schemaVersion: EVIDENCE_SCHEMA_VERSION,
    evidenceBoundary: EVIDENCE_BOUNDARY,
    nonProductionNotice:
      "Deterministic synthetic UI evidence against a conforming Connect fake; does not replace hosted sign-in/provider or live Cloud Run release acceptance.",
    revision: opts.revision,
    distSha256: opts.distSha256,
    beforeRevision: isComparative ? opts.beforeRevision : null,
    beforeDistSha256: isComparative ? opts.beforeDistSha256 : null,
    command: opts.command,
    flow: opts.flow,
    mode: opts.mode,
    syntheticDataOnly: true,
    seedState: opts.seedState,
    viewports: viewportEntries,
  };
  const manifestPath = path.join(opts.outputDir, "manifest.json");
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`, "utf8");
  return { manifestPath, manifest };
}

function assertExpectedField(actual, expected, label) {
  if (expected && actual !== expected) {
    throw new Error(
      `video-evidence: ${label} mismatch (got ${String(actual)}, expected ${expected})`,
    );
  }
}

function validateManifestHeader(
  manifest,
  rawText,
  {
    expectedRevision,
    expectedBeforeRevision,
    expectedDistSha256,
    expectedBeforeDistSha256,
    expectedFlow,
  },
) {
  for (const pattern of FORBIDDEN_SECRET_PATTERNS) {
    if (pattern.test(rawText)) {
      throw new Error("video-evidence: forbidden secret detected in manifest");
    }
  }
  if (manifest.schemaVersion !== EVIDENCE_SCHEMA_VERSION) {
    throw new Error(`video-evidence: unsupported schemaVersion ${String(manifest.schemaVersion)}`);
  }
  if (manifest.evidenceBoundary !== EVIDENCE_BOUNDARY || manifest.syntheticDataOnly !== true) {
    throw new Error("video-evidence: manifest must declare synthetic deterministic UI boundary");
  }
  validateRevisionBindings(
    manifest.mode,
    manifest.revision,
    manifest.beforeRevision,
    manifest.distSha256,
    manifest.beforeDistSha256,
  );
  assertExpectedField(manifest.revision, expectedRevision, "revision");
  assertExpectedField(manifest.beforeRevision, expectedBeforeRevision, "beforeRevision");
  assertExpectedField(manifest.distSha256, expectedDistSha256, "distSha256");
  assertExpectedField(manifest.beforeDistSha256, expectedBeforeDistSha256, "beforeDistSha256");
  assertExpectedField(manifest.flow, expectedFlow, "flow");
}

async function validateArtifactDigest(rootDir, entry, maxBytes) {
  if (!SHA256_RE.test(entry?.sha256 ?? "") || !Number.isInteger(entry?.bytes)) {
    throw new Error("video-evidence: artifact metadata missing bytes or sha256");
  }
  const { bytes } = await resolveEvidenceFile(rootDir, entry.path);
  if (bytes.length !== entry.bytes || sha256Hex(bytes) !== entry.sha256) {
    throw new Error(`video-evidence: digest or byte length mismatch for ${entry.path}`);
  }
  if (bytes.length > maxBytes) {
    throw new Error(
      `video-evidence: ${entry.path} exceeds byte bound (${bytes.length} > ${maxBytes})`,
    );
  }
  return bytes;
}

function assertTrackMatchesViewport(track, videoMeta, viewport) {
  if (!track.codecId.startsWith("V_")) {
    throw new Error(`video-evidence: unexpected WebM video codec ${track.codecId}`);
  }
  if (track.width !== viewport.width || track.height !== viewport.height) {
    throw new Error(
      `video-evidence: video track dimensions ${track.width}x${track.height} != ${viewport.width}x${viewport.height}`,
    );
  }
  if (videoMeta.width !== viewport.width || videoMeta.height !== viewport.height) {
    throw new Error(`video-evidence: video metadata dimensions mismatch viewport ${viewport.name}`);
  }
}

async function validateCapture(
  rootDir,
  viewport,
  capture,
  expectedPhaseRevision,
  expectedPhaseDistSha,
  limits,
) {
  if (!capture || typeof capture !== "object") {
    throw new Error(`video-evidence: missing capture for viewport ${viewport.name}`);
  }
  if (capture.revision !== expectedPhaseRevision) {
    throw new Error(
      `video-evidence: capture revision mismatch for ${viewport.name} (got ${String(capture.revision)}, expected ${expectedPhaseRevision})`,
    );
  }
  if (expectedPhaseDistSha !== null && capture.distSha256 !== expectedPhaseDistSha) {
    throw new Error(
      `video-evidence: capture distSha256 mismatch for ${viewport.name} (got ${String(capture.distSha256)}, expected ${expectedPhaseDistSha})`,
    );
  }
  const videoBytes = await validateArtifactDigest(rootDir, capture.video, limits.maxVideoBytes);
  const track = parseWebmVideoTrack(videoBytes, limits.minVideoBytes);
  assertTrackMatchesViewport(track, capture.video, viewport);
  if (!Array.isArray(capture.screenshots) || capture.screenshots.length === 0) {
    throw new Error(`video-evidence: capture for ${viewport.name} has no screenshots`);
  }
  let totalBytes = videoBytes.length;
  for (const shot of capture.screenshots) {
    const shotBytes = await validateArtifactDigest(rootDir, shot, limits.maxScreenshotBytes);
    const dims = parsePngDimensions(shotBytes);
    if (dims.width !== viewport.width || dims.height !== viewport.height) {
      throw new Error(
        `video-evidence: screenshot ${shot.path} dimensions ${dims.width}x${dims.height} != ${viewport.width}x${viewport.height}`,
      );
    }
    totalBytes += shotBytes.length;
  }
  return { totalBytes, screenshotCount: capture.screenshots.length };
}

function validateBeforeAfterPair(viewport, captures) {
  const before = captures?.before;
  const after = captures?.after;
  if (!before || !after) {
    throw new Error(
      `video-evidence: viewport ${viewport.name} missing matched before/after captures`,
    );
  }
  if (before.startingStateHash !== after.startingStateHash) {
    throw new Error(
      `video-evidence: viewport ${viewport.name} before/after startingStateHash mismatch`,
    );
  }
  const beforeSteps = before.screenshots.map((item) => item.step).join(",");
  const afterSteps = after.screenshots.map((item) => item.step).join(",");
  if (beforeSteps !== afterSteps) {
    throw new Error(
      `video-evidence: viewport ${viewport.name} before/after screenshot steps mismatch`,
    );
  }
}

async function validateViewportEntry(
  rootDir,
  viewport,
  requiredPhases,
  manifest,
  requireBeforeAfter,
  limits,
) {
  if (requireBeforeAfter) {
    validateBeforeAfterPair(viewport, viewport.captures);
  }
  let totalBytes = 0;
  let videoCount = 0;
  let screenshotCount = 0;
  for (const phase of requiredPhases) {
    const phaseRevision = phase === "before" ? manifest.beforeRevision : manifest.revision;
    const phaseDistSha =
      phase === "before" ? (manifest.beforeDistSha256 ?? null) : (manifest.distSha256 ?? null);
    const checked = await validateCapture(
      rootDir,
      viewport,
      viewport.captures?.[phase],
      phaseRevision,
      phaseDistSha,
      limits,
    );
    totalBytes += checked.totalBytes;
    videoCount += 1;
    screenshotCount += checked.screenshotCount;
  }
  return { totalBytes, videoCount, screenshotCount, isMobile: viewport.width <= 480 };
}

function validateManifestShape(manifest, requireBeforeAfter) {
  if (!Array.isArray(manifest.viewports) || manifest.viewports.length === 0) {
    throw new Error("video-evidence: manifest declares no viewports");
  }
  if (requireBeforeAfter && manifest.mode !== "before-after") {
    throw new Error("video-evidence: before-after evidence is required");
  }
  return resolvePhases(manifest.mode);
}

async function assertEvidenceTotals(summary, requireMobile, effectiveLimits, rootDir) {
  if (requireMobile && !summary.sawMobile) {
    throw new Error("video-evidence: mobile viewport capture is required");
  }
  if (summary.totalBytes > effectiveLimits.maxTotalBytes) {
    throw new Error(
      `video-evidence: total evidence size ${summary.totalBytes} exceeds ${effectiveLimits.maxTotalBytes}`,
    );
  }
  const rootEntries = await readdir(rootDir);
  if (!rootEntries.includes("manifest.json")) {
    throw new Error("video-evidence: manifest.json missing from evidence directory");
  }
}

export async function validateVideoEvidence({
  manifestPath,
  expectedRevision = null,
  expectedBeforeRevision = null,
  expectedDistSha256 = null,
  expectedBeforeDistSha256 = null,
  expectedFlow = null,
  requireBeforeAfter = false,
  requireMobile = false,
  limits = DEFAULT_LIMITS,
}) {
  const effectiveLimits = { ...DEFAULT_LIMITS, ...limits };
  const resolvedManifest = path.resolve(manifestPath);
  const rootDir = path.dirname(resolvedManifest);
  const rawText = await readFile(resolvedManifest, "utf8");
  const manifest = JSON.parse(rawText);
  validateManifestHeader(manifest, rawText, {
    expectedRevision,
    expectedBeforeRevision,
    expectedDistSha256,
    expectedBeforeDistSha256,
    expectedFlow,
  });
  const requiredPhases = validateManifestShape(manifest, requireBeforeAfter);

  const summary = {
    totalBytes: Buffer.byteLength(rawText, "utf8"),
    videoCount: 0,
    screenshotCount: 0,
    sawMobile: false,
  };
  for (const viewport of manifest.viewports) {
    const part = await validateViewportEntry(
      rootDir,
      viewport,
      requiredPhases,
      manifest,
      requireBeforeAfter,
      effectiveLimits,
    );
    summary.totalBytes += part.totalBytes;
    summary.videoCount += part.videoCount;
    summary.screenshotCount += part.screenshotCount;
    summary.sawMobile = summary.sawMobile || part.isMobile;
  }
  await assertEvidenceTotals(summary, requireMobile, effectiveLimits, rootDir);
  return { ok: true, manifest, ...summary };
}
