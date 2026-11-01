// The pinned browser starts with only pinned libraries, runs a script, and lays out and paints text
// with only pinned fonts (specs/005-browser-client FR-006, FR-013).
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";

import { chromium } from "@playwright/test";

import { browserLaunchOptions } from "./stack.mjs";

// Form controls matter. Without fonts, a plain paragraph paints nothing and exits 0, and only a
// control's font lookup reaches the crash.
const PAGE =
  '<!doctype html><h1 id="a">before</h1><label>Name <input value="text"></label><button>Save</button>';

const fontsUsed = async (page, selector) => {
  const session = await page.context().newCDPSession(page);
  await session.send("DOM.enable");
  await session.send("CSS.enable");
  const { root } = await session.send("DOM.getDocument");
  const { nodeId } = await session.send("DOM.querySelector", { nodeId: root.nodeId, selector });
  const { fonts } = await session.send("CSS.getPlatformFontsForNode", { nodeId });
  return fonts.map((font) => font.familyName);
};

// Canvas pixels, because a screenshot can exist with no text in it.
const inkedPixels = (page) =>
  page.evaluate(() => {
    const canvas = document.createElement("canvas");
    canvas.width = 200;
    canvas.height = 60;
    const context = canvas.getContext("2d");
    context.font = "32px sans-serif";
    context.fillText("Painted", 4, 40);
    const { data } = context.getImageData(0, 0, canvas.width, canvas.height);
    return data.filter((value, index) => index % 4 === 3 && value > 0).length;
  });

const options = browserLaunchOptions();

// The named failure on a host with no font data. The same command runs twice, and only the sysroot
// differs: pinned, then empty, which also hides any fonts this host has. Host fonts can match the
// pinned family, so the sysroot itself is checked.
const scratch = mkdtempSync(path.join(process.env.TEST_TMPDIR ?? tmpdir(), "fonts-"));
const render = (sysroot) =>
  spawnSync(
    options.executablePath,
    [
      "--no-sandbox",
      "--disable-gpu",
      `--user-data-dir=${path.join(scratch, path.basename(sysroot))}`,
      `--screenshot=${path.join(scratch, "render.png")}`,
      `data:text/html,${encodeURIComponent(PAGE)}`,
    ],
    { encoding: "utf8", env: { ...options.env, FONTCONFIG_SYSROOT: sysroot }, timeout: 30000 },
  );

assert.ok(
  existsSync(path.join(options.env.FONTCONFIG_SYSROOT ?? "", "etc/fonts/fonts.conf")),
  "the browser is not launched with the pinned font data",
);
const pinned = render(options.env.FONTCONFIG_SYSROOT);
assert.equal(pinned.status, 0, pinned.stderr);
assert.doesNotMatch(pinned.stderr, /Fontconfig error|FATAL/);

const missing = render(mkdtempSync(path.join(scratch, "empty-")));
assert.notEqual(missing.status, 0, "the page rendered with no font data");
assert.match(missing.stderr, /Fontconfig error: Cannot load default config file/);
assert.match(
  missing.stderr,
  /FATAL:third_party\/skia\/src\/ports\/SkFontMgr_FontConfigInterface\.cpp:\d+\] Not implemented/,
);

const browser = await chromium.launch(options);
assert.equal(browser.version(), "156.0.8078.4");
const page = await browser.newPage();
await page.setContent(PAGE);
await page.evaluate(() => {
  document.getElementById("a").textContent = "after";
});
assert.equal(await page.textContent("#a"), "after");
assert.deepEqual(await fontsUsed(page, "#a"), ["DejaVu Sans"]);
assert.ok((await page.locator("#a").boundingBox()).width > 0, "text has no width");
assert.ok((await inkedPixels(page)) > 0, "text painted no pixels");
const withText = await page.screenshot();
await page.evaluate(() => {
  document.body.replaceChildren();
});
assert.notDeepEqual(await page.screenshot(), withText, "the screenshot does not show the text");
await browser.close();

console.log("browser smoke passed");
