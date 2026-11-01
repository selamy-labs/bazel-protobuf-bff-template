// Captures the local demo's journey as frames (spec 012 FR-024): the real page served by the running launcher,
// the real server behind it, the pinned browser driving it. Nothing is mocked; the frames are what the browser
// painted. Run by tools/dev/record.sh against a launcher it started.
//
//   capture.mjs <frames directory> <page origin> <subject>
//
// Each frame is a PNG with a short caption in its file name; the assembler orders them by number.
import { mkdirSync } from "node:fs";
import path from "node:path";

import { chromium } from "@playwright/test";

const [framesDir, origin, subject] = process.argv.slice(2);
if (!framesDir || !origin || !subject) {
  console.error("usage: capture.mjs <frames directory> <page origin> <subject>");
  process.exit(2);
}
const browserDir = path.resolve(process.env.BROWSER_DIR ?? "");
if (!process.env.BROWSER_DIR) {
  console.error("BROWSER_DIR is not set; run this through bazel run //tools/dev/record:capture");
  process.exit(2);
}
mkdirSync(framesDir, { recursive: true });

let index = 0;
const frame = async (page, caption) => {
  index += 1;
  const file = path.join(framesDir, `${String(index).padStart(2, "0")}-${caption}.png`);
  await page.screenshot({ path: file, fullPage: false });
  console.log(`frame ${index}: ${caption}`);
};

const browser = await chromium.launch({
  executablePath: path.join(browserDir, "chrome-headless-shell"),
  env: {
    ...process.env,
    LD_LIBRARY_PATH: path.join(browserDir, "lib"),
    FONTCONFIG_SYSROOT: path.join(browserDir, "fontconfig-root"),
  },
});
try {
  const page = await browser.newPage({
    viewport: { width: 720, height: 480 },
    deviceScaleFactor: 1,
  });
  await page.goto(origin);
  await page.getByTestId("test-identity").waitFor();
  await frame(page, "open");
  await page.getByLabel("Test identity").fill(subject);
  await frame(page, "sign-in");
  await page.getByLabel("Title").fill("Shopping");
  await page.getByLabel("Body").fill("milk, bread");
  await frame(page, "typed");
  await page.getByRole("button", { name: "Create note" }).click();
  await page.getByRole("heading", { name: "Shopping" }).waitFor();
  await frame(page, "created");
  const name = await page.getByTestId("note-name").textContent();
  await page.reload();
  await page.getByTestId("test-identity").waitFor();
  await frame(page, "reloaded");
  await page.getByLabel("Test identity").fill(subject);
  await page.getByLabel("Name").fill(name);
  await frame(page, "name-typed");
  await page.getByRole("button", { name: "Open note" }).click();
  await page.getByTestId("note-body").waitFor();
  await frame(page, "opened");
  console.log(`captured ${index} frames of ${name} in ${framesDir}`);
} finally {
  await browser.close();
}
