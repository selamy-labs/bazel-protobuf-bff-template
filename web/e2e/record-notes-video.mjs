// Single-command template recording target for the Notes reference flow (#27).
// Runs the built web client (using the generated Connect client) against the conforming stateful
// Connect fake, writes matched screenshots, Playwright WebM videos, and an exact-revision manifest,
// then validates the emitted bundle fail-closed.
import path from "node:path";
import { fileURLToPath } from "node:url";

import { serve } from "../serve/serve.mjs";
import { startStatefulNotesConnectFake } from "./connect-notes-fake.mjs";
import {
  VIEWPORTS,
  hashDirectoryTree,
  recordWebFlowEvidence,
  validateVideoEvidence,
  verifyDistProvenance,
} from "./video-evidence.mjs";

export const NOTES_REFERENCE_FLOW = "notes-reference-flow";

export const NOTES_SYNTHETIC_SEED = Object.freeze({
  subjects: ["ana", "bob", "fault-unavailable"],
  initialNotes: [
    {
      name: "notes/seed-1",
      owner: "ana",
      title: "Baseline note",
      body: "Synthetic seed for matched before/after starting state.",
      createTimeIso: "2026-01-01T00:00:00.000Z",
    },
  ],
});

async function identify(page, subject) {
  await page.getByLabel("Test identity").fill(subject);
}

async function createNote(page, title, body) {
  await page.getByLabel("Title").fill(title);
  await page.getByLabel("Body").fill(body);
  await page.getByRole("button", { name: "Create note" }).click();
}

async function openNote(page, name) {
  await page.getByLabel("Name").fill(name);
  await page.getByRole("button", { name: "Open note" }).click();
}

export async function runNotesReferenceFlow({ page, capture, webDist, seedState }) {
  const site = await serve(webDist, 0, "/");
  const fake = await startStatefulNotesConnectFake({
    allowedOrigin: site.origin,
    initialNotes: seedState.initialNotes,
  });
  site.setBffUrl(fake.url);
  try {
    await page.goto(site.origin);
    await page.getByTestId("test-identity").waitFor();
    await identify(page, "ana");
    await openNote(page, "notes/seed-1");
    await page.getByRole("heading", { name: "Baseline note" }).waitFor();
    await capture("start-seeded");

    await createNote(page, "Release checklist", "Verify state, reload, denial, and failure.");
    await page.getByRole("heading", { name: "Release checklist" }).waitFor();
    const createdName = (await page.getByTestId("note-name").textContent())?.trim();
    await capture("created");

    await page.reload();
    await page.getByTestId("test-identity").waitFor();
    await identify(page, "ana");
    await openNote(page, createdName);
    await page.getByTestId("note-body").waitFor();
    await capture("reloaded-opened");

    await createNote(page, "   ", "");
    await page.getByTestId("title-error").waitFor();
    await capture("validation-error");

    await identify(page, "bob");
    await openNote(page, createdName);
    await page.getByTestId("failure").waitFor();
    await capture("cross-subject-denied");

    await identify(page, "fault-unavailable");
    await createNote(page, "Transient check", "expects unavailable");
    await page.getByTestId("failure").waitFor();
    await capture("service-unavailable");
  } finally {
    await fake.close();
    await site.close();
  }
}

const CLI_OPTION_KEYS = {
  "--out": "outDir",
  "--revision": "revision",
  "--before-revision": "beforeRevision",
  "--before-tree": "beforeTree",
  "--before-web-dist": "beforeWebDist",
  "--before-provenance": "beforeProvenance",
  "--mode": "mode",
  "--viewports": "viewportsRaw",
  "--command": "command",
};

export function parseCliArgs(argv) {
  const parsed = {
    outDir: "",
    revision: process.env.BFF_VIDEO_REVISION ?? "",
    beforeRevision: "",
    beforeTree: "",
    beforeWebDist: "",
    beforeProvenance: "",
    mode: "representative-smoke",
    viewportsRaw: "desktop,mobile",
    command: "tools/checks/web_video_evidence.sh",
  };
  for (let i = 0; i < argv.length; i += 2) {
    const key = CLI_OPTION_KEYS[argv[i]];
    if (!key || i + 1 >= argv.length) {
      throw new Error(`record-notes-video: unknown or incomplete argument ${argv[i]}`);
    }
    parsed[key] = argv[i + 1];
  }
  return {
    ...parsed,
    viewports: parsed.viewportsRaw.split(",").filter(Boolean),
  };
}

export async function resolveVerifiedBeforeBuild(options) {
  const baselineValues = [
    options.beforeRevision,
    options.beforeTree,
    options.beforeWebDist,
    options.beforeProvenance,
  ];
  if (options.mode === "representative-smoke") {
    if (baselineValues.some(Boolean)) {
      throw new Error("record-notes-video: baseline flags are only valid with --mode before-after");
    }
    return { beforeDist: null, beforeDistSha256: null };
  }
  if (baselineValues.some((value) => !value)) {
    throw new Error(
      "record-notes-video: --mode before-after requires --before-revision, --before-tree, --before-web-dist, and --before-provenance",
    );
  }
  const beforeDist = path.resolve(options.beforeWebDist);
  const { distSha256 } = await verifyDistProvenance({
    distDir: beforeDist,
    provenancePath: options.beforeProvenance,
    expectedRevision: options.beforeRevision,
    expectedTree: options.beforeTree,
    expectedTarget: "//web/app:dist_dev",
  });
  return { beforeDist, beforeDistSha256: distSha256 };
}

async function runCli() {
  const options = parseCliArgs(process.argv.slice(2));
  const webDist = process.env.WEB_DIST;
  if (!webDist) {
    throw new Error("record-notes-video: WEB_DIST is not set; run through Bazel");
  }
  if (!options.outDir) {
    throw new Error("record-notes-video: --out <dir> is required");
  }
  const { beforeDist, beforeDistSha256 } = await resolveVerifiedBeforeBuild(options);
  const afterDistSha256 = await hashDirectoryTree(webDist);
  const selectedViewports = options.viewports.map((name) => {
    const viewport = VIEWPORTS[name];
    if (!viewport) {
      throw new Error(`record-notes-video: unknown viewport ${name}`);
    }
    return viewport;
  });
  const beforeRevision = options.mode === "before-after" ? options.beforeRevision : null;
  const { manifestPath } = await recordWebFlowEvidence({
    outputDir: path.resolve(options.outDir),
    revision: options.revision,
    beforeRevision,
    distSha256: afterDistSha256,
    beforeDistSha256,
    command: options.command,
    flow: NOTES_REFERENCE_FLOW,
    mode: options.mode,
    viewports: selectedViewports,
    seedState: NOTES_SYNTHETIC_SEED,
    runFlow: ({ page, capture, phase, seedState }) =>
      runNotesReferenceFlow({
        page,
        capture,
        webDist: phase === "before" ? beforeDist : webDist,
        seedState,
      }),
  });
  const verified = await validateVideoEvidence({
    manifestPath,
    expectedRevision: options.revision,
    expectedBeforeRevision: beforeRevision,
    expectedDistSha256: afterDistSha256,
    expectedBeforeDistSha256: beforeDistSha256,
    expectedFlow: NOTES_REFERENCE_FLOW,
    requireBeforeAfter: options.mode === "before-after",
    requireMobile: selectedViewports.some((v) => v.width <= 480),
  });
  console.log(
    `video-evidence: verified ${verified.videoCount} videos and ${verified.screenshotCount} screenshots (${verified.totalBytes} bytes) in ${path.dirname(manifestPath)}`,
  );
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await runCli();
}
